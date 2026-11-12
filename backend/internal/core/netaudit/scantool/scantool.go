// Package scantool exposes the network scan service to the agent as the network_scan tool. It turns a model's call
// into a Request, runs it through the service, and returns the result as a labelled block of untrusted data.
package scantool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/findingsview"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

// Scanner is the part of netaudit.Service the tool uses.
type Scanner interface {
	Scan(ctx context.Context, req netaudit.Request) (netaudit.Result, error)
}

// AckSource lists a workspace's accepted findings. *persistence.WorkspaceManager satisfies it.
type AckSource interface {
	ReadAcks(workspaceID string) ([]models.ScanAck, error)
}

// Module builds the tool module. available hides the tool while it returns false (see netaudit.Readiness.Blocking).
// acks, when given, keeps the findings the operator accepted out of what the model is shown. digest, when given, returns the
// record of the audit's window (netaudit.DigestSource.Digest); the tool adds it after the scan in its own fence, and a run it
// returns "" for (an ordinary chat) gets the scan alone.
func Module(scanner Scanner, acks AckSource, available func(context.Context) bool, digest func(context.Context, *netaudit.CurrentScan) string) toolregistry.Module {
	return toolregistry.Module{
		Handlers: map[string]toolregistry.ToolHandler{
			models.ToolNetworkScan: toolregistry.Handle(func(ctx context.Context, args scanArgs) (any, error) {
				return run(ctx, scanner, acks, digest, args)
			}),
		},
		Available: available,
	}
}

// targetList reads the targets as an array of strings, or as one comma-separated string — the second form is what
// some models send for a list.
type targetList []string

func (t *targetList) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*t = list
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return fmt.Errorf("targets must be a list of strings or one comma-separated string")
	}
	for _, part := range strings.Split(single, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*t = append(*t, part)
		}
	}
	return nil
}

type scanArgs struct {
	Targets targetList `json:"targets"`
	Mode    string     `json:"mode"`
	Ports   string     `json:"ports"`
	// Thorough is optional; false is the quick scan.
	Thorough bool `json:"thorough"`
}

func run(ctx context.Context, scanner Scanner, acks AckSource, digest func(context.Context, *netaudit.CurrentScan) string, args scanArgs) (any, error) {
	req := netaudit.Request{Targets: args.Targets, Mode: netaudit.Mode(args.Mode), Ports: args.Ports, Thorough: args.Thorough}
	// A network audit has its own settings, kept with the automation: they decide what is scanned and how, and nothing
	// the model passes changes them. A run without settings (an ordinary chat or automation) uses its arguments.
	if cfg, ok := models.ScanSettingsFrom(ctx); ok {
		req = netaudit.RequestFromSettings(cfg)
	}
	req.Unattended = models.IsUnattendedRun(ctx)
	res, err := scanner.Scan(ctx, req)
	if err != nil {
		return nil, explain(err)
	}
	// An automation run builds the observed part of its report from these, not from what the model says it saw.
	if collector := netaudit.CollectorFrom(ctx); collector != nil {
		collector.Add(res)
	}
	shown, hidden := res, 0
	if acks != nil {
		if accepted, err := acks.ReadAcks(models.GetWorkspaceID(ctx)); err == nil {
			shown, hidden = netaudit.HideAccepted(res, accepted, time.Now())
		}
	}
	text, err := render(req.Mode, shown, hidden)
	if err != nil || digest == nil {
		return text, err
	}
	if d := digest(ctx, netaudit.CurrentOf(res)); d != "" {
		text += "\n\n" + prompts.FenceScanData(d)
	}
	return text, nil
}

// explain turns a refusal that only the operator can fix into ErrToolUnavailable, so the loop stops retrying; every
// other error is model-actionable (a bad target, a bad port list) and stays plain.
func explain(err error) error {
	operatorFixable := errors.Is(err, scanscope.ErrScanningOff) || errors.Is(err, netaudit.ErrNmapRequired) ||
		errors.Is(err, isolation.ErrUnconfined) || errors.Is(err, scanscope.ErrOutOfScope)
	if !operatorFixable {
		return err
	}
	return &models.ToolUnavailableError{Reason: err.Error(), Cause: err}
}

// render writes a short header, the result as JSON inside the fenced block, and the rule for reading the block.
func render(mode netaudit.Mode, res netaudit.Result, hidden int) (string, error) {
	payload := struct {
		Engine   string             `json:"engine"`
		Hosts    []netaudit.Host    `json:"hosts"`
		Findings []netaudit.Finding `json:"findings"`
		Warnings []string           `json:"warnings,omitempty"`
	}{res.Engine, answered(res.Hosts), res.Findings, res.Warnings}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode the scan result: %w", err)
	}
	header := fmt.Sprintf("network_scan %s finished with %s: %d device(s) answered of %d address(es) looked at; only the devices that answered are listed. %s", mode, res.Engine, devicesUp(res.Hosts), len(res.Hosts), findingsview.Line(res.Findings))
	if hidden > 0 {
		header += fmt.Sprintf(" %d finding(s) the operator has accepted are left out.", hidden)
	}
	return header + "\n" + prompts.FenceScanData(string(body)), nil
}

// answered leaves out the addresses that did not answer: a /24 scan lists all 254 of them, which is thousands of tokens of
// {"up":false} that bury the devices and invite the model to invent ranges. The header still says how many addresses were looked at,
// and a device that was not finished (unscanned) stays, because its ports are unknown.
func answered(hosts []netaudit.Host) []netaudit.Host {
	out := make([]netaudit.Host, 0, len(hosts))
	for _, h := range hosts {
		if h.Up || h.Unscanned || len(h.Ports) > 0 {
			out = append(out, h)
		}
	}
	return out
}

// devicesUp counts the hosts that answered. The count is typed, so the model does not have to count the list, which holds every
// address looked at and shows ports only for the ones that had any.
func devicesUp(hosts []netaudit.Host) int {
	n := 0
	for _, h := range hosts {
		if h.Up {
			n++
		}
	}
	return n
}
