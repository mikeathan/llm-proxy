package netaudit

import (
	"fmt"
	"strings"
	"time"
)

// Observed is everything the code-rendered part of a report states.
type Observed struct {
	ScannedAt time.Time
	// Previous is when the earlier scan was made; zero when this is the first.
	Previous time.Time
	Result   Result
	Delta    Delta
	// Accepted is how many findings of this scan an operator's acceptance covers; they are counted, not listed.
	Accepted int
}

// ObservedHeading titles the section the code writes. The model's text follows it under its own heading.
const (
	ObservedHeading   = "## Observed (generated from the scan data, not written by the model)"
	AssessmentHeading = "## Assessment (written by the model)"
	timeLayout        = "2006-01-02 15:04 MST"
)

// RenderObserved writes the facts of a scan as markdown: how many addresses were looked at, and how the findings differ
// from the previous scan. It is deterministic and uses no model output; every value from the network is escaped so it
// cannot break the table or smuggle markup into the report.
func RenderObserved(o Observed) string {
	var b strings.Builder
	b.WriteString(ObservedHeading + "\n\n")

	up := 0
	for _, h := range o.Result.Hosts {
		if h.Up {
			up++
		}
	}
	fmt.Fprintf(&b, "Scanned %d address(es) on %s with %s: %d up, %d down.\n",
		len(o.Result.Hosts), o.ScannedAt.Format(timeLayout), cell(o.Result.Engine), up, len(o.Result.Hosts)-up)
	for _, w := range o.Result.Warnings {
		fmt.Fprintf(&b, "\n> Note: %s\n", cell(w))
	}

	if unscanned := unscannedAddrs(o.Result.Hosts); len(unscanned) > 0 {
		fmt.Fprintf(&b, "\n> %d live address(es) were **not scanned to completion**, so their ports are unknown (not \"none open\") "+
			"and they were left out of the comparison: %s.\n", len(unscanned), cell(strings.Join(firstN(unscanned, maxListedAddrs), ", ")))
	}

	if o.Accepted > 0 {
		fmt.Fprintf(&b, "\n%d finding(s) in this scan are accepted by the operator and not listed below.\n", o.Accepted)
	}
	if o.Previous.IsZero() {
		fmt.Fprintf(&b, "\nFirst scan: there is no earlier result to compare with, so all %d open port(s)%s are recorded as the baseline.\n",
			countChecks(o.Result.Findings, false), checkCountNote(o.Result.Findings))
	} else {
		fmt.Fprintf(&b, "\nCompared with the scan of %s: %d new, %d resolved, %d changed, %d unchanged.\n",
			o.Previous.Format(timeLayout), len(o.Delta.New), len(o.Delta.Resolved), len(o.Delta.Changed), o.Delta.Unchanged)
	}
	if o.Previous.IsZero() {
		writeTable(&b, "Open ports", o.Result.Findings)
		writeChecks(&b, "Checks", o.Result.Findings)
		return strings.TrimRight(b.String(), "\n")
	}
	writeTable(&b, "New", o.Delta.New)
	writeChecks(&b, "New checks", o.Delta.New)
	writeTable(&b, "Resolved", o.Delta.Resolved)
	writeChecks(&b, "Resolved checks", o.Delta.Resolved)
	if len(o.Delta.Changed) > 0 {
		b.WriteString("\n### Changed\n\n| Device | Port | Before | After |\n|---|---|---|---|\n")
		for _, c := range o.Delta.Changed {
			fmt.Fprintf(&b, "| %s | %d/%s | %s | %s |\n", cell(c.After.Device), c.After.Port, cell(c.After.Proto), cell(c.Before.Version), cell(c.After.Version))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// maxListedAddrs bounds how many addresses a note names.
const maxListedAddrs = 12

func unscannedAddrs(hosts []Host) []string {
	var out []string
	for _, h := range hosts {
		if h.Unscanned {
			out = append(out, h.Addr.String())
		}
	}
	return out
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(items[:n:n], fmt.Sprintf("and %d more", len(items)-n))
}

// countChecks counts the findings that are checks (true) or open ports (false).
func countChecks(findings []Finding, checks bool) int {
	n := 0
	for _, f := range findings {
		if (f.Check != CheckOpenPort) == checks {
			n++
		}
	}
	return n
}

// checkCountNote is ", and N check finding(s)," when the scan ran checks that found something, else nothing.
func checkCountNote(findings []Finding) string {
	if n := countChecks(findings, true); n > 0 {
		return fmt.Sprintf(" and %d check finding(s)", n)
	}
	return ""
}

// writeChecks lists the check findings among findings: what a safe script learned about a service. The evidence is the
// code's own wording (see checks.go), escaped like every other cell.
func writeChecks(b *strings.Builder, title string, findings []Finding) {
	if countChecks(findings, true) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n| Device | Port | Severity | Finding | Suggested fix |\n|---|---|---|---|---|\n", title)
	for _, f := range findings {
		if f.Check != CheckOpenPort {
			fmt.Fprintf(b, "| %s | %d/%s | %s | %s | %s |\n", cell(f.Device), f.Port, cell(f.Proto), cell(f.Severity), cell(f.Evidence), cell(RemediationFor(f.Check)))
		}
	}
}

// writeTable lists the open ports among findings.
func writeTable(b *strings.Builder, title string, findings []Finding) {
	if countChecks(findings, false) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n| Device | Port | Service | Version |\n|---|---|---|---|\n", title)
	for _, f := range findings {
		if f.Check == CheckOpenPort {
			fmt.Fprintf(b, "| %s | %d/%s | %s | %s |\n", cell(f.Device), f.Port, cell(f.Proto), cell(f.Service), cell(f.Version))
		}
	}
}

// cell makes text from the network safe inside a markdown table cell: one line, no pipes, no backticks, no HTML.
func cell(s string) string {
	r := strings.NewReplacer("|", "/", "`", "'", "<", "(", ">", ")", "\r", " ", "\n", " ", "\t", " ")
	return strings.TrimSpace(r.Replace(s))
}

// SummaryOf is the one-line account of a scan a connector message carries: counts only. No address, port, service or
// version from the network appears in it, so nothing a device wrote can reach a chat channel through it.
func SummaryOf(o Observed, name string) string {
	up := 0
	for _, h := range o.Result.Hosts {
		if h.Up {
			up++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Network audit %s: %d of %d addresses up", name, up, len(o.Result.Hosts))
	if o.Previous.IsZero() {
		fmt.Fprintf(&b, "; first scan, %d open port(s)%s recorded as the baseline", countChecks(o.Result.Findings, false), checkCountNote(o.Result.Findings))
	} else {
		fmt.Fprintf(&b, "; %d new, %d changed, %d resolved", len(o.Delta.New), len(o.Delta.Changed), len(o.Delta.Resolved))
	}
	if o.Accepted > 0 {
		fmt.Fprintf(&b, "; %d accepted", o.Accepted)
	}
	if n := len(unscannedAddrs(o.Result.Hosts)); n > 0 {
		fmt.Fprintf(&b, "; %d not scanned to completion", n)
	}
	b.WriteString(". Open the app for the details.")
	return b.String()
}
