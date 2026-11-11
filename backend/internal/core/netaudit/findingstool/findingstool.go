// Package findingstool exposes the saved result of the network audit to the assistant as the network_findings tool. It reads
// what the Findings screen reads (findingsview) and hands the model a short, bounded summary: fixed wording in the header, and
// every piece of text a device or the operator wrote inside the untrusted-data fence.
package findingstool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/findingsview"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/models"
)

// Listing bounds. A dense result overflowed a 16k-token context once, so the lists are cut worst first and the cut is stated.
const (
	maxListed   = 25
	maxResolved = 10
)

// Source reads one audit's report. findingsview.Reader satisfies it.
type Source interface {
	Read(workspaceID, automation string) (findingsview.Report, error)
}

// Module builds the tool module. audits names the network audits of a workspace; it is resolved on the server for the run's
// own workspace, so the model never names a workspace or an audit. The tool is offered only where an audit exists.
func Module(src Source, audits func(workspaceID string) []string) toolregistry.Module {
	return toolregistry.Module{
		Handlers: map[string]toolregistry.ToolHandler{
			models.ToolNetworkFindings: toolregistry.Handle(func(ctx context.Context, _ struct{}) (any, error) {
				ws := models.GetWorkspaceID(ctx)
				return summarize(src, ws, audits(ws))
			}),
		},
		Available: func(ctx context.Context) bool { return len(audits(models.GetWorkspaceID(ctx))) > 0 },
	}
}

// listed is one finding as the model sees it. It carries no guide, no fix text and no operator note: the model explains a
// finding from its check title and evidence, and the operator's own words are not facts about the network.
type listed struct {
	Device          string `json:"device"`
	Port            int    `json:"port,omitempty"`
	Proto           string `json:"proto,omitempty"`
	Check           string `json:"check"`
	Severity        string `json:"severity"`
	Service         string `json:"service,omitempty"`
	Version         string `json:"version,omitempty"`
	Evidence        string `json:"evidence,omitempty"`
	New             bool   `json:"new,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	FixState        string `json:"fix_state,omitempty"`
	// FirstSeen and CameBack are the finding's history: how long it has been there, and how often it returned after going.
	FirstSeen string `json:"first_seen,omitempty"`
	CameBack  int    `json:"came_back,omitempty"`
}

type gone struct {
	Device  string `json:"device"`
	Port    int    `json:"port,omitempty"`
	Check   string `json:"check"`
	FixedAt string `json:"marked_fixed_at,omitempty"`
}

func summarize(src Source, workspaceID string, audits []string) (string, error) {
	if len(audits) == 0 {
		return "", fmt.Errorf("no network audit is set up in this workspace")
	}
	latest, scanned, err := latestOf(src, workspaceID, audits)
	if err != nil {
		return "", err
	}
	if !scanned {
		return "network_findings: the network audit has not scanned yet, so there is nothing to report.", nil
	}
	return render(latest, len(audits))
}

// latestOf reads every audit of the workspace and keeps the one scanned last. A store that cannot be read is an error: it must
// never read as a clean network.
func latestOf(src Source, workspaceID string, audits []string) (findingsview.Report, bool, error) {
	var best findingsview.Report
	var bestAt time.Time
	found := false
	for _, name := range audits {
		rep, err := src.Read(workspaceID, name)
		if err != nil {
			return best, false, fmt.Errorf("failed to read the network audit result: %w", err)
		}
		at, perr := time.Parse(time.RFC3339, rep.ScannedAt)
		if !rep.Scanned || perr != nil {
			continue
		}
		if !found || at.After(bestAt) {
			best, bestAt, found = rep, at, true
		}
	}
	return best, found, nil
}

func render(rep findingsview.Report, auditCount int) (string, error) {
	open, accepted := openFindings(rep.Findings)
	shown, more := rank(open)
	payload := struct {
		ScannedAt string   `json:"scanned_at"`
		FirstScan bool     `json:"first_scan,omitempty"`
		DevicesUp int      `json:"devices_up"`
		Unscanned []string `json:"unscanned,omitempty"`
		Findings  []listed `json:"findings"`
		More      int      `json:"more"`
		Resolved  []gone   `json:"resolved,omitempty"`
	}{rep.ScannedAt, rep.FirstScan, rep.DevicesUp, rep.Unscanned, shown, more, goneOf(rep.Resolved)}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode the network audit result: %w", err)
	}
	return header(rep, open, accepted, auditCount) + "\n" + prompts.FenceScanData(string(body)), nil
}

// header is fixed wording and numbers only: nothing a device or the operator wrote can reach it.
func header(rep findingsview.Report, open []findingsview.Finding, accepted, auditCount int) string {
	plain := make([]netaudit.Finding, len(open))
	newCount := 0
	for i, f := range open {
		plain[i] = f.ScanFinding
		if f.New {
			newCount++
		}
	}
	h := fmt.Sprintf("network_findings: the latest audit scan finished %s. %s", rep.ScannedAt, findingsview.Line(plain))
	if rep.FirstScan {
		h += " This was the first scan, so nothing in it counts as new."
	} else {
		h += fmt.Sprintf(" %d new since the scan before; %d no longer shown.", newCount, len(rep.Resolved))
	}
	if accepted > 0 {
		h += fmt.Sprintf(" %d finding(s) the operator has accepted are left out.", accepted)
	}
	if auditCount > 1 {
		h += fmt.Sprintf(" This workspace has %d audits; this is the one scanned last.", auditCount)
	}
	return h
}

// openFindings drops the accepted findings and counts them.
func openFindings(all []findingsview.Finding) (open []findingsview.Finding, accepted int) {
	for _, f := range all {
		switch {
		case f.Accepted:
			accepted++
		case f.VerifiedFixedAt != "":
			// A recheck found it gone since the scan; the next audit confirms it. It is not an open problem.
		default:
			open = append(open, f)
		}
	}
	return open, accepted
}

// rank keeps what needs the operator, worst first: every check finding, and an open port only when it is new. The rest is
// counted in the header and cut at maxListed.
func rank(open []findingsview.Finding) ([]listed, int) {
	var pick []findingsview.Finding
	for _, f := range open {
		if f.Check != netaudit.CheckOpenPort || f.New {
			pick = append(pick, f)
		}
	}
	sort.SliceStable(pick, func(i, j int) bool {
		if a, b := severityRank(pick[i].Severity), severityRank(pick[j].Severity); a != b {
			return a > b
		}
		return pick[i].New && !pick[j].New
	})
	more := 0
	if len(pick) > maxListed {
		more, pick = len(pick)-maxListed, pick[:maxListed]
	}
	out := make([]listed, len(pick))
	for i, f := range pick {
		out[i] = listed{
			Device: f.Device, Port: f.Port, Proto: f.Proto, Check: titleOr(f), Severity: f.Severity, Service: f.Service,
			Version: f.Version, Evidence: f.Evidence, New: f.New, PreviousVersion: f.PreviousVersion, FixState: f.FixState,
			FirstSeen: f.FirstSeen, CameBack: f.CameBack,
		}
	}
	return out, more
}

func titleOr(f findingsview.Finding) string {
	if f.CheckTitle != "" {
		return f.CheckTitle
	}
	return f.Check
}

// checkName is the check's plain title, or its id when it has none (an open port).
func checkName(check string) string {
	if t := netaudit.CheckTitle(check); t != "" {
		return t
	}
	return check
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case netaudit.SeverityCritical:
		return 4
	case netaudit.SeverityHigh:
		return 3
	case netaudit.SeverityMedium:
		return 2
	case netaudit.SeverityLow:
		return 1
	}
	return 0
}

func goneOf(resolved []findingsview.Resolved) []gone {
	if len(resolved) > maxResolved {
		resolved = resolved[:maxResolved]
	}
	out := make([]gone, len(resolved))
	for i, r := range resolved {
		out[i] = gone{Device: r.Device, Port: r.Port, Check: checkName(r.Check), FixedAt: r.FixedAt}
	}
	return out
}
