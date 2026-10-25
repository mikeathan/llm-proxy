package netaudit

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

var reportTime = time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)

func hostsWith(findings ...Finding) []Host {
	byDevice := map[string]*Host{}
	var order []string
	for _, f := range findings {
		h, ok := byDevice[f.Device]
		if !ok {
			h = &Host{Addr: netip.MustParseAddr(f.Device), Up: true}
			byDevice[f.Device] = h
			order = append(order, f.Device)
		}
		h.Ports = append(h.Ports, Port{Number: f.Port, Proto: f.Proto, State: "open", Service: f.Service, Version: f.Version})
	}
	var out []Host
	for _, d := range order {
		out = append(out, *byDevice[d])
	}
	return out
}

func TestRenderObserved_FirstScanRecordsABaseline(t *testing.T) {
	f := finding("10.0.0.1", 22, "OpenSSH 8.9")
	got := RenderObserved(Observed{ScannedAt: reportTime, Result: Result{Engine: "nmap", Hosts: hostsWith(f), Findings: []Finding{f}}})
	for _, want := range []string{ObservedHeading, "Scanned 1 address(es)", "with nmap: 1 up, 0 down", "First scan", "### Open ports", "| 10.0.0.1 | 22/tcp |"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderObserved_ComparisonListsNewResolvedAndChanged(t *testing.T) {
	prev := []Finding{finding("10.0.0.1", 22, "OpenSSH 8.9"), finding("10.0.0.2", 445, "")}
	cur := []Finding{finding("10.0.0.1", 22, "OpenSSH 9.6"), finding("10.0.0.3", 3306, "")}
	got := RenderObserved(Observed{
		ScannedAt: reportTime, Previous: reportTime.Add(-24 * time.Hour),
		Result: Result{Engine: "nmap", Hosts: hostsWith(cur...), Findings: cur}, Delta: Diff(prev, cur),
	})
	for _, want := range []string{"1 new, 1 resolved, 1 changed, 0 unchanged", "### New", "10.0.0.3", "### Resolved", "10.0.0.2", "### Changed", "OpenSSH 8.9 | OpenSSH 9.6"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderObserved_NothingFromTheNetworkCanBreakTheTable(t *testing.T) {
	hostile := finding("10.0.0.1", 80, "x | y\n## Injected `code` <script>alert(1)</script>")
	hostile.Service = "evil|svc"
	got := RenderObserved(Observed{ScannedAt: reportTime, Result: Result{Engine: "nmap", Hosts: hostsWith(hostile), Findings: []Finding{hostile}}})
	for _, bad := range []string{"\n## Injected", "<script>", "`code`"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q survived in the report:\n%s", bad, got)
		}
	}
	row := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "| 10.0.0.1") {
			row = line
		}
	}
	if strings.Count(row, "|") != 5 {
		t.Errorf("a finding must stay one row of four cells, got %q", row)
	}
}

func TestRenderObserved_IsDeterministic(t *testing.T) {
	f := []Finding{finding("10.0.0.1", 22, ""), finding("10.0.0.2", 80, "")}
	o := Observed{ScannedAt: reportTime, Previous: reportTime.Add(-time.Hour), Result: Result{Engine: "nmap", Hosts: hostsWith(f...), Findings: f}, Delta: Diff(nil, f)}
	if RenderObserved(o) != RenderObserved(o) {
		t.Error("the same input must give the same text")
	}
}
