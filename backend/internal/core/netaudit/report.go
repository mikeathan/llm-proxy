package netaudit

import (
	"fmt"
	"slices"
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
	// Vulns says how far known-vulnerability matching reached; the zero value means it was not asked for.
	Vulns VulnCoverage
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
	writeCoverage(&b, o.Vulns)
	if o.Previous.IsZero() {
		fmt.Fprintf(&b, "\nFirst scan: there is no earlier result to compare with, so all %d open port(s)%s are recorded as the baseline.\n",
			countChecks(o.Result.Findings, false), checkCountNote(o.Result.Findings))
	} else {
		fmt.Fprintf(&b, "\nCompared with the scan of %s: %d new, %d resolved, %d changed, %d unchanged.\n",
			o.Previous.Format(timeLayout), len(o.Delta.New), len(o.Delta.Resolved), len(o.Delta.Changed), o.Delta.Unchanged)
	}
	if o.Previous.IsZero() {
		writeInventory(&b, "Open ports", o.Result.Findings)
		writeChecks(&b, "Checks", o.Result.Findings, true)
		return strings.TrimRight(b.String(), "\n")
	}
	writeTable(&b, "New", o.Delta.New)
	writeChecks(&b, "New checks", o.Delta.New, true)
	writeTable(&b, "Resolved", o.Delta.Resolved)
	writeChecks(&b, "Resolved checks", o.Delta.Resolved, false)
	if len(o.Delta.Changed) > 0 {
		b.WriteString("\n### Changed\n\n| Device | Port | Before | After |\n|---|---|---|---|\n")
		for _, c := range o.Delta.Changed {
			fmt.Fprintf(&b, "| %s | %d/%s | %s | %s |\n", cell(c.After.Device), c.After.Port, cell(c.After.Proto), cell(changeText(c.Before)), cell(changeText(c.After)))
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

// writeChecks lists the check findings among findings: what a safe script or a port rule learned. The same problem on
// several services is one row that names where it is, and each distinct check's suggested fix is written once under the
// table, so a long report does not repeat a paragraph per service. The evidence is the code's own wording (see
// checks.go), escaped like every other cell.
func writeChecks(b *strings.Builder, title string, findings []Finding, withFixes bool) {
	groups := groupChecks(findings)
	if len(groups) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n| Severity | Finding | Where |\n|---|---|---|\n", title)
	for _, g := range groups {
		fmt.Fprintf(b, "| %s | %s | %s |\n", cell(g.severity), cell(g.evidence), cell(strings.Join(firstN(g.where, maxListedAddrs), ", ")))
	}
	if !withFixes {
		return
	}
	b.WriteString("\nSuggested fixes:\n\n")
	seen := map[string]bool{}
	for _, g := range groups {
		if fix := RemediationFor(g.check); fix != "" && !seen[g.check] {
			seen[g.check] = true
			fmt.Fprintf(b, "- **%s**: %s\n", cell(CheckTitle(g.check)), cell(fix))
		}
	}
}

// checkGroup is one distinct problem and every place it was found.
type checkGroup struct {
	check, severity, evidence string
	where                     []string
}

// groupChecks merges the check findings that say the same thing (same check, severity and words) and orders the result
// worst first; within a severity, in the order the findings came.
func groupChecks(findings []Finding) []checkGroup {
	var groups []checkGroup
	index := map[[3]string]int{}
	for _, f := range findings {
		if f.Check == CheckOpenPort {
			continue
		}
		key := [3]string{f.Check, f.Severity, f.Evidence}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, checkGroup{check: f.Check, severity: f.Severity, evidence: f.Evidence})
		}
		groups[i].where = append(groups[i].where, fmt.Sprintf("%s %d/%s", f.Device, f.Port, f.Proto))
	}
	slices.SortStableFunc(groups, func(a, b checkGroup) int { return severityRank(b.severity) - severityRank(a.severity) })
	return groups
}

// severityOrder ranks severities from the mildest; an unknown one ranks below them all.
var severityOrder = []string{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}

func severityRank(s string) int { return slices.Index(severityOrder, s) }

// writeInventory lists the open ports of a first scan as one row per device, so a baseline of dozens of ports stays a
// handful of lines. Later scans list only what changed (writeTable).
func writeInventory(b *strings.Builder, title string, findings []Finding) {
	var devices []string
	ports := map[string][]string{}
	for _, f := range findings {
		if f.Check != CheckOpenPort {
			continue
		}
		if _, ok := ports[f.Device]; !ok {
			devices = append(devices, f.Device)
		}
		ports[f.Device] = append(ports[f.Device], portSummary(f))
	}
	if len(devices) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n| Device | Open ports |\n|---|---|\n", title)
	for _, d := range devices {
		fmt.Fprintf(b, "| %s | %s |\n", cell(d), cell(strings.Join(ports[d], "; ")))
	}
}

// portSummary is "22/tcp ssh OpenSSH 9.6p1": the port, then whatever the scan could say about the service behind it.
func portSummary(f Finding) string {
	return strings.TrimSpace(fmt.Sprintf("%d/%s %s %s", f.Port, f.Proto, f.Service, f.Version))
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
	b.WriteString(coverageSummary(o.Vulns))
	if n := len(unscannedAddrs(o.Result.Hosts)); n > 0 {
		fmt.Fprintf(&b, "; %d not scanned to completion", n)
	}
	b.WriteString(". Open the app for the details.")
	return b.String()
}

// changeText is what the Changed table shows for a finding: its version, and for a known_vulns finding the number of
// flaws too, because a new flaw can arrive with the version unchanged.
func changeText(f Finding) string {
	if f.Check != CheckKnownVulns {
		return f.Version
	}
	noun := "flaws"
	if len(f.CVEs) == 1 {
		noun = "flaw"
	}
	return fmt.Sprintf("%s (%d possible %s)", f.Version, len(f.CVEs), noun)
}
