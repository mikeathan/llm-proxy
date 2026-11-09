package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openPortOf(device string, port int, service string) Finding {
	return Finding{Device: device, Port: port, Proto: "tcp", Service: service, Check: CheckOpenPort, Severity: SeverityInfo}
}

func TestWebTargets_AreTheWebPortsOfAScanAndNothingElse(t *testing.T) {
	fs := []Finding{
		openPortOf("192.168.1.20", 80, "http"), openPortOf("192.168.1.20", 443, "https"), openPortOf("192.168.1.20", 8443, "ssl/http"),
		openPortOf("192.168.1.20", 8080, "http-proxy (guess)"), openPortOf("192.168.1.20", 22, "ssh"), openPortOf("192.168.1.20", 53, "domain"),
		openPortOf("192.168.1.30", 80, "http"), {Device: "192.168.1.20", Port: 9000, Proto: "udp", Service: "http", Check: CheckOpenPort},
		{Device: "192.168.1.20", Port: 81, Proto: "tcp", Service: "http", Check: CheckTelnetOpen}, openPortOf(HostDevice, 80, "http"),
		openPortOf("8.8.8.8", 80, "http"),
	}
	got := WebTargets(fs, "")
	var urls []string
	for _, tg := range got {
		urls = append(urls, fmt.Sprintf("%s://%s:%d", map[bool]string{false: "http", true: "https"}[tg.TLS], tg.Addr, tg.Port))
	}
	want := []string{"http://192.168.1.20:80", "https://192.168.1.20:443", "https://192.168.1.20:8443", "http://192.168.1.20:8080", "http://192.168.1.30:80"}
	if strings.Join(urls, " ") != strings.Join(want, " ") {
		t.Fatalf("targets = %v, want %v", urls, want)
	}
	if one := WebTargets(fs, "192.168.1.30"); len(one) != 1 || one[0].Addr.String() != "192.168.1.30" {
		t.Fatalf("one device = %+v", one)
	}
}

func TestWebTargets_AreBoundedPerDeviceAndInAll(t *testing.T) {
	var fs []Finding
	for p := 80; p < 80+3*maxWebPortsPerDevice; p++ {
		fs = append(fs, openPortOf("192.168.1.20", p, "http"))
	}
	if got := WebTargets(fs, ""); len(got) != maxWebPortsPerDevice {
		t.Fatalf("%d targets for one device, want %d", len(got), maxWebPortsPerDevice)
	}
}

func hit(id string, port int) WebHit {
	return WebHit{ID: id, Addr: mustAddr("192.168.1.20"), Port: port}
}

func TestWebFindings_OneFindingPerWebServiceInOurOwnWords(t *testing.T) {
	got := WebFindings([]WebHit{hit("grafana_login", 80), hit("apache_status_page", 80), hit("grafana_login", 8080)})
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want one for each of the two ports", got)
	}
	f := got[0]
	if f.Device != "192.168.1.20" || f.Port != 80 || f.Proto != "tcp" || f.Check != CheckWebExposure || f.Severity != SeverityLow {
		t.Fatalf("finding = %+v, want the more serious of the two hits' severity", f)
	}
	if !strings.Contains(f.Evidence, "The login page of Grafana") || !strings.Contains(f.Evidence, "An Apache status page that anyone can open") || f.Version == "" {
		t.Fatalf("evidence = %q version = %q", f.Evidence, f.Version)
	}
	if checkTitles[CheckWebExposure] == "" || remediations[CheckWebExposure] == "" {
		t.Fatal("the check needs a title and a fix")
	}
}

// Acceptance of a finding holds on its Version, a digest of the sorted check ids, and the evidence is the titles in the order the hits arrive.
// Pinned exactly (values taken from the code before the web checks moved into the Network stack, ADR-0038) so the move cannot change a
// finding's words, severity or version, which would read as resolved-and-new and lose the operator's acknowledgements.
func TestWebFindings_AreByteIdenticalToWhatTheyWereBeforeTheMove(t *testing.T) {
	got := WebFindings([]WebHit{hit("apache_status_page", 80), hit("grafana_login", 80)})
	want := Finding{
		Device: "192.168.1.20", Port: 80, Proto: "tcp", Check: CheckWebExposure, Severity: SeverityLow,
		Evidence: "The web checks found: An Apache status page that anyone can open; The login page of Grafana", Version: "f11970191815",
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) || got[0].Key() != want.Key() {
		t.Fatalf("finding = %+v, want exactly %+v", got, want)
	}
}

// The words and the severity of a hit come from the core's own table: an id the table does not know is dropped, so nothing the Network stack
// says beyond a known id reaches a finding.
func TestWebFindings_AnIdTheCoreDoesNotKnowIsDropped(t *testing.T) {
	if got := WebFindings([]WebHit{hit("some_unknown_check", 80)}); len(got) != 0 {
		t.Fatalf("an unknown id made a finding: %+v", got)
	}
	got := WebFindings([]WebHit{hit("some_unknown_check", 80), hit("grafana_login", 80)})
	if len(got) != 1 || strings.Contains(got[0].Evidence, "unknown") {
		t.Fatalf("findings = %+v", got)
	}
}

// Every check has a title and a severity the audit spells, and a bound the stack's own is pinned equal to.
func TestWebChecks_TheTableIsCompleteAndTheBoundMatchesTheSDK(t *testing.T) {
	if len(webChecks) == 0 {
		t.Fatal("no checks")
	}
	for _, c := range webChecks {
		if c.ID == "" || c.Title == "" || severityRank(c.Severity) == 0 && c.Severity != SeverityInfo {
			t.Errorf("check %+v needs an id, a title and a severity the audit spells", c)
		}
	}
	if MaxWebTargets != 64 {
		t.Errorf("MaxWebTargets = %d: it must equal the stack's bound (stacksdk.MaxWebTargets, checked in app)", MaxWebTargets)
	}
}

type fakeWeb struct {
	limit   time.Duration
	targets []WebTarget
	res     WebResult
	err     error
}

func (f *fakeWeb) ProbeTLS(context.Context, []WebTarget, time.Duration) (TLSResult, error) {
	return TLSResult{}, nil
}

func (f *fakeWeb) Probe(_ context.Context, t []WebTarget, limit time.Duration) (WebResult, error) {
	f.targets, f.limit = t, limit
	return f.res, f.err
}

func TestRunReporter_RunsTheWebChecksOnlyWhileTheModuleIsOn(t *testing.T) {
	on := false
	web := &fakeWeb{res: WebResult{Hits: []WebHit{hit("grafana_login", 80)}}}
	r := NewRunReporter(nil).WithWebChecks(func() bool { return on }, web)
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 80, "http")}, Hosts: []Host{{Addr: mustAddr("192.168.1.20"), Up: true}}}
	if note := r.addWebChecks(context.Background(), &result); note != "" || len(web.targets) != 0 {
		t.Fatalf("note = %q targets = %v while the module is off", note, web.targets)
	}
	on = true
	if note := r.addWebChecks(context.Background(), &result); note != "" || len(web.targets) != 1 || len(result.Findings) != 2 {
		t.Fatalf("note = %q targets = %v findings = %+v", note, web.targets, result.Findings)
	}
}

func TestRunReporter_SaysWhenTheWebChecksCouldNotRunAndRecordsTheFailure(t *testing.T) {
	web := &fakeWeb{err: errors.New("outside the scan scope")}
	health := NewRunHealth()
	r := NewRunReporter(nil).WithWebChecks(func() bool { return true }, web).WithRunHealth(health)
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 80, "http")}}
	before := len(result.Findings)
	if note := r.addWebChecks(context.Background(), &result); !strings.Contains(note, "web checks could not run") || len(result.Findings) != before {
		t.Fatalf("note = %q: a failed run is said, never read as clean", note)
	}
	if got := health.Reading(MonitorWebChecks, true, time.Hour); !got.Failed {
		t.Fatalf("reading = %+v, want the failure recorded", got)
	}
	web.err = nil
	r.addWebChecks(context.Background(), &result)
	if got := health.Reading(MonitorWebChecks, true, time.Hour); got.Failed || got.LastGood.IsZero() {
		t.Fatalf("reading = %+v, want a run that worked to clear the failure", got)
	}
}

func TestRunReporter_ARunWithNoWebServicesCountsAsWorking(t *testing.T) {
	health := NewRunHealth()
	r := NewRunReporter(nil).WithWebChecks(func() bool { return true }, &fakeWeb{}).WithRunHealth(health)
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 22, "ssh")}}
	r.addWebChecks(context.Background(), &result)
	if got := health.Reading(MonitorWebChecks, true, time.Hour); got.Failed || got.LastGood.IsZero() {
		t.Fatalf("reading = %+v", got)
	}
}

func TestRunReporter_AShutDownIsNotAFailureOfTheWebChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	health := NewRunHealth()
	r := NewRunReporter(nil).WithWebChecks(func() bool { return true }, &fakeWeb{err: ctx.Err()}).WithRunHealth(health)
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 80, "http")}}
	r.addWebChecks(ctx, &result)
	if got := health.Reading(MonitorWebChecks, true, time.Hour); got.Failed {
		t.Fatalf("reading = %+v: stopping the program is not a failed run", got)
	}
}

func TestDeepChecks_AddTheWebChecksOfTheDevicesWebPortsWhenReady(t *testing.T) {
	base := baselineWith(openPortOf("192.168.1.20", 80, "http"), openPortOf("192.168.1.20", 22, "ssh"))
	d := newDeepChecks(&fakeDeepScanner{}, base, &memDeepStore{})
	web := &fakeWeb{res: WebResult{Hits: []WebHit{hit("grafana_login", 80)}}}
	d.WithWebChecks(web)
	check, err := d.Run(context.Background(), "ws", "audit", "192.168.1.20")
	if err != nil {
		t.Fatal(err)
	}
	if len(web.targets) != 1 || web.targets[0].Port != 80 {
		t.Fatalf("targets = %+v, want only the web port", web.targets)
	}
	if web.limit <= 0 || web.limit > 3*time.Minute {
		t.Fatalf("limit = %v: the deep check answers an HTTP request, so its web step is short", web.limit)
	}
	found := false
	for _, f := range check.Findings {
		found = found || f.Check == CheckWebExposure
	}
	if !found {
		t.Fatalf("findings = %+v, want the web finding", check.Findings)
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestWebChecksConfig_IsOffUntilSwitchedOn(t *testing.T) {
	store := &memConfig{}
	c := NewWebChecksConfig(store)
	if c.Enabled() {
		t.Fatal("the nightly web checks touch every web page on the network, so they are off until the operator turns them on")
	}
	if err := c.Set(true); err != nil || !c.Enabled() || !store.Get().WebChecksEnabled {
		t.Errorf("on: err=%v enabled=%v", err, c.Enabled())
	}
}
