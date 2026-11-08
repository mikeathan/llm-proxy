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
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

// Fences around the data block. The model is told, beside the block, that nothing inside it is an instruction.
const (
	dataOpen  = "<untrusted-scan-data>"
	dataClose = "</untrusted-scan-data>"
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
// acks, when given, keeps the findings the operator accepted out of what the model is shown.
func Module(scanner Scanner, acks AckSource, available func(context.Context) bool) toolregistry.Module {
	return toolregistry.Module{
		Handlers: map[string]toolregistry.ToolHandler{
			models.ToolNetworkScan: toolregistry.Handle(func(ctx context.Context, args scanArgs) (any, error) {
				return run(ctx, scanner, acks, args)
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

func run(ctx context.Context, scanner Scanner, acks AckSource, args scanArgs) (any, error) {
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
	return render(req.Mode, shown, hidden)
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

// noCheckFindings is what the header says when the checks established nothing. An open port is a fact, not a problem; a model
// told only a total of "findings" counted ports as problems and named checks it had read about in the tool description.
const noCheckFindings = "No check found a problem in this scan: do not name one."

// findingsLine counts the open ports apart from what the checks established, and names each check once, in fixed words, with
// its count, in the order first seen.
func findingsLine(findings []netaudit.Finding) string {
	ports, checks := 0, 0
	counts := map[string]int{}
	var order []string
	for _, f := range findings {
		if f.Check == netaudit.CheckOpenPort {
			ports++
			continue
		}
		checks++
		if counts[f.Check] == 0 {
			order = append(order, f.Check)
		}
		counts[f.Check]++
	}
	line := fmt.Sprintf("%d open port(s).", ports)
	if checks == 0 {
		return line + " " + noCheckFindings
	}
	named := make([]string, 0, len(order))
	for _, c := range order {
		title := netaudit.CheckTitle(c)
		if title == "" {
			title = c
		}
		named = append(named, fmt.Sprintf("%s (%d)", title, counts[c]))
	}
	return fmt.Sprintf("%s %d check finding(s): %s.", line, checks, strings.Join(named, "; "))
}

// render writes a short header, the result as JSON inside the fenced block, and the rule for reading the block.
func render(mode netaudit.Mode, res netaudit.Result, hidden int) (string, error) {
	payload := struct {
		Engine   string             `json:"engine"`
		Hosts    []netaudit.Host    `json:"hosts"`
		Findings []netaudit.Finding `json:"findings"`
		Warnings []string           `json:"warnings,omitempty"`
	}{res.Engine, res.Hosts, res.Findings, res.Warnings}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode the scan result: %w", err)
	}
	// A banner could contain the closing fence; break it so the block cannot be ended from the inside.
	data := strings.ReplaceAll(string(body), dataClose, "</untrusted-scan-data (removed)>")
	header := fmt.Sprintf("network_scan %s finished with %s: %d host(s). %s", mode, res.Engine, len(res.Hosts), findingsLine(res.Findings))
	if hidden > 0 {
		header += fmt.Sprintf(" %d finding(s) the operator has accepted are left out.", hidden)
	}
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s", header, dataOpen, data, dataClose, prompts.UntrustedScanDataNote), nil
}
