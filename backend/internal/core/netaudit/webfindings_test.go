package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/webprobe"
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

func hit(id, title, sev string, port int) webprobe.Hit {
	return webprobe.Hit{ID: id, Addr: mustAddr("192.168.1.20"), Port: port, Severity: sev, Title: title}
}

func TestWebFindings_OneFindingPerWebServiceInOurOwnWords(t *testing.T) {
	got := WebFindings([]webprobe.Hit{hit("a", "Page A", "info", 80), hit("b", "Page B", "low", 80), hit("a", "Page A", "info", 8080)})
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want one for each of the two ports", got)
	}
	f := got[0]
	if f.Device != "192.168.1.20" || f.Port != 80 || f.Proto != "tcp" || f.Check != CheckWebExposure || f.Severity != SeverityLow {
		t.Fatalf("finding = %+v, want the more serious of the two hits' severity", f)
	}
	if !strings.Contains(f.Evidence, "Page A") || !strings.Contains(f.Evidence, "Page B") || f.Version == "" {
		t.Fatalf("evidence = %q version = %q", f.Evidence, f.Version)
	}
	if checkTitles[CheckWebExposure] == "" || remediations[CheckWebExposure] == "" {
		t.Fatal("the check needs a title and a fix")
	}
}

type fakeWeb struct {
	limit   time.Duration
	targets []webprobe.Target
	res     webprobe.Result
	err     error
}

func (f *fakeWeb) Probe(_ context.Context, t []webprobe.Target, limit time.Duration) (webprobe.Result, error) {
	f.targets, f.limit = t, limit
	return f.res, f.err
}

func TestRunReporter_RunsTheWebChecksOnlyWhileTheModuleIsOn(t *testing.T) {
	on := false
	web := &fakeWeb{res: webprobe.Result{Hits: []webprobe.Hit{hit("a", "Page A", "info", 80)}}}
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
	web := &fakeWeb{res: webprobe.Result{Hits: []webprobe.Hit{hit("a", "Page A", "info", 80)}}}
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
