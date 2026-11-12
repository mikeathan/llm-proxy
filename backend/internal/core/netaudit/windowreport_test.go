package netaudit

import (
	"context"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeWindow struct {
	d  WindowDigest
	ok bool
}

func (f fakeWindow) Build(context.Context, string, string, *CurrentScan) (WindowDigest, bool) {
	return f.d, f.ok
}

func TestRenderObserved_AFindingThatStaysOpenIsListedOnEveryRepeatScan(t *testing.T) {
	high := Finding{Device: "this-computer", Check: "host_file_modified", Severity: SeverityHigh, Evidence: "A program file on this computer no longer matches its package"}
	fresh := Finding{Device: "10.0.0.5", Port: 23, Proto: "tcp", Check: CheckTelnetOpen, Severity: SeverityHigh, Evidence: "Telnet is open"}
	port := Finding{Device: "10.0.0.5", Port: 23, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo}
	o := Observed{
		ScannedAt: ackNow, Previous: ackNow.Add(-time.Hour),
		Result: Result{Engine: "nmap", Findings: []Finding{high, fresh, port}},
		Delta:  Delta{New: []Finding{fresh, port}, Unchanged: 1},
	}
	out := RenderObserved(o)
	still := out[strings.Index(out, "### Still open"):]
	if !strings.Contains(out, "### Still open") || !strings.Contains(still, "no longer matches its package") || !strings.Contains(still, "| high |") {
		t.Fatalf("a high finding that did not change must still be in the report:\n%s", out)
	}
	if strings.Contains(still, "Telnet is open") {
		t.Errorf("a finding listed as new is not listed again as standing:\n%s", still)
	}
	if !strings.Contains(out, "### New checks") {
		t.Errorf("the new one stays where it was:\n%s", out)
	}
}

func TestRenderObserved_AFirstScanHasNoStandingSectionAndNothingStandingMeansNone(t *testing.T) {
	f := Finding{Device: "this-computer", Check: "host_firewall_off", Severity: SeverityLow, Evidence: "The firewall is off"}
	first := RenderObserved(Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap", Findings: []Finding{f}}})
	if strings.Contains(first, "Still open") || !strings.Contains(first, "### Checks") {
		t.Errorf("a first scan lists every check in its own table:\n%s", first)
	}
	repeat := RenderObserved(Observed{ScannedAt: ackNow, Previous: ackNow.Add(-time.Hour), Result: Result{Engine: "nmap"}})
	if strings.Contains(repeat, "Still open") {
		t.Errorf("no standing problem, no section:\n%s", repeat)
	}
}

func TestRenderObserved_TheWindowSectionIsWrittenByCodeAndEscaped(t *testing.T) {
	d := WindowDigest{
		Window: "24h", AsOf: ackNow.Add(-time.Hour),
		Scans:        DigestScans{Count: 4, DevicesUp: DigestRange{15, 16}, OpenPorts: DigestRange{37, 38}},
		GoneInWindow: []DigestChange{{Device: "10.0.0.2", Port: 49152, Check: "open_port"}}, GoneMore: 2,
		ReturnedInWindow: []DigestChange{{Device: "10.0.0.3|x", Check: "A kernel module was loaded"}},
		OpenAllWindow:    3,
		Activity:         DigestActivity{PortsOpened: 3, PortsClosed: 1, RouterBlocked: 10, Alerts: map[string]int{"new_device": 2}},
		Unread:           []string{"alert log"},
	}
	out := RenderObserved(Observed{ScannedAt: ackNow, Previous: ackNow.Add(-time.Hour), Result: Result{Engine: "nmap"}, Window: &d})
	for _, want := range []string{
		"### Last 24h before this scan", "4 scan(s) recorded: devices up 15 to 16, open ports 37 to 38",
		"Went away: 10.0.0.2 port 49152 (open port); and 2 more.", "Came back: 10.0.0.3/x (A kernel module was loaded).",
		"3 finding(s) were already open", "3 port(s) opened, 1 port(s) closed, 10 connection attempt(s) the router refused, 2 new_device alert(s)",
		"Not read, so not counted: alert log",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "x |") || strings.Contains(out, "3|x") {
		t.Errorf("device text must not break the markup:\n%s", out)
	}
	if empty := RenderObserved(Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap"}, Window: &WindowDigest{}}); strings.Contains(empty, "Last ") {
		t.Errorf("a window with nothing recorded and no earlier scan adds no section:\n%s", empty)
	}
	none := RenderObserved(Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap"}, Window: &WindowDigest{Window: "24h", AsOf: ackNow.Add(-time.Hour)}})
	if !strings.Contains(none, "The scan log has no scan in this window; the latest saved scan is from") {
		t.Errorf("an empty scan log beside a saved scan says so:\n%s", none)
	}
}

// The regression this section exists for: the vertex report of 2026-11-12 14:48 listed no standing problem at all, so a high finding
// open for a day was nowhere in it and the model, told the report had it, left it out too.
func TestRunReporter_ARepeatReportKeepsTheStandingHighFindingAndTheWindow(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	high := Finding{Device: "10.0.0.9", Port: 23, Proto: "tcp", Check: CheckTelnetOpen, Severity: SeverityHigh, Evidence: "Telnet is open: a remote login that sends passwords unencrypted"}
	open := Finding{Device: "10.0.0.9", Port: 23, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo}
	run := func(at time.Time) string {
		ctx, c := WithCollector(context.Background())
		c.Add(Result{Engine: "nmap", Hosts: hostsWith(open), Findings: []Finding{open, high}})
		return reporter(store, at).WithWindow(fakeWindow{ok: true, d: WindowDigest{Window: "24h", AsOf: at, Scans: DigestScans{Count: 1, DevicesUp: DigestRange{1, 1}, OpenPorts: DigestRange{1, 1}}}}).Augment(ctx, "ws", "nightly", "")
	}
	first := run(reportTime)
	second := run(reportTime.Add(24 * time.Hour))
	if !strings.Contains(first, "### Checks") {
		t.Fatalf("the first scan lists its checks:\n%s", first)
	}
	if !strings.Contains(second, "### Still open") || !strings.Contains(second, "| high | ") || !strings.Contains(second, "remote login that sends passwords") || !strings.Contains(second, "### Last 24h before this scan") {
		t.Errorf("the second, unchanged scan must still show the high finding and the window:\n%s", second)
	}
	if strings.Contains(second, "### New checks") {
		t.Errorf("nothing is new on the second scan:\n%s", second)
	}
}

// The run's own context carries the audit's name and scan settings but not the workspace (a tool call adds that), and the report is
// written from that context. The window must read the records of the workspace and audit the reporter is told, or it reads none and
// the section is silently empty.
func TestRunReporter_TheWindowIsReadForTheWorkspaceTheReporterIsToldNotOneFromTheContext(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	digest := DigestSource{Store: fakeDigestStore{log: []models.ScanEntry{
		{Automation: "nightly", At: reportTime.Add(-2 * time.Hour), DevicesUp: 15, OpenPorts: 36},
	}, base: models.ScanBaseline{ScannedAt: reportTime.Add(-2 * time.Hour)}}, Now: func() time.Time { return reportTime }}

	ctx, c := WithCollector(models.WithScanSettings(models.WithTaskName(context.Background(), "nightly"), models.ScanSettings{Window: models.ScanWindowDay}))
	c.Add(Result{Engine: "nmap", Hosts: hostsWith(finding("10.0.0.1", 22, "OpenSSH 9"))})
	got := reporter(store, reportTime).WithWindow(digest).Augment(ctx, "ws", "nightly", "")
	if !strings.Contains(got, "### Last 24h before this scan") || !strings.Contains(got, "1 scan(s) recorded: devices up 15, open ports 36") {
		t.Errorf("the window must be built from the workspace and audit the reporter was given:\n%s", got)
	}

	plain, pc := WithCollector(context.Background())
	pc.Add(Result{Engine: "nmap", Hosts: hostsWith(finding("10.0.0.1", 22, "OpenSSH 9"))})
	if out := reporter(&memBaselines{data: map[string]models.ScanBaseline{}}, reportTime).WithWindow(digest).Augment(plain, "ws", "nightly", ""); strings.Contains(out, "### Last ") {
		t.Errorf("a run with no scan settings is not an audit and gets no window:\n%s", out)
	}
}
