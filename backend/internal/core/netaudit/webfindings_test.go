package netaudit

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/nuclei"
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
		urls = append(urls, tg.URL())
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

func hit(id, title, sev string, port int) nuclei.Hit {
	return nuclei.Hit{TemplateID: id, Addr: mustAddr("192.168.1.20"), Port: port, Severity: sev, Title: title}
}

func TestWebFindings_OneFindingPerWebServiceInOurOwnWords(t *testing.T) {
	got := WebFindings([]nuclei.Hit{hit("a", "Page A", "info", 80), hit("b", "Page B", "low", 80), hit("a", "Page A", "info", 8080)})
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
	ok      bool
	targets []nuclei.Target
	res     nuclei.Result
	err     error
}

func (f *fakeWeb) Ready() (bool, string) { return f.ok, "nuclei is not installed" }
func (f *fakeWeb) Check(_ context.Context, t []nuclei.Target, _ time.Duration) (nuclei.Result, error) {
	f.targets = t
	return f.res, f.err
}

func TestRunReporter_RunsTheWebChecksOnlyWhileTheModuleIsOnAndReady(t *testing.T) {
	on := false
	web := &fakeWeb{ok: true, res: nuclei.Result{Hits: []nuclei.Hit{hit("a", "Page A", "info", 80)}}}
	r := NewRunReporter(nil).WithWebChecks(func() bool { return on }, web)
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 80, "http")}, Hosts: []Host{{Addr: mustAddr("192.168.1.20"), Up: true}}}
	if note := r.addWebChecks(context.Background(), &result); note != "" || len(web.targets) != 0 {
		t.Fatalf("note = %q targets = %v while the module is off", note, web.targets)
	}
	on = true
	if note := r.addWebChecks(context.Background(), &result); note != "" || len(web.targets) != 1 || len(result.Findings) != 2 {
		t.Fatalf("note = %q targets = %v findings = %+v", note, web.targets, result.Findings)
	}
	web.ok = false
	before := len(result.Findings)
	if note := r.addWebChecks(context.Background(), &result); !strings.Contains(note, "web checks") || len(result.Findings) != before {
		t.Fatalf("note = %q: an unready checker is said, never read as clean", note)
	}
}

func TestDeepChecks_AddTheWebChecksOfTheDevicesWebPortsWhenReady(t *testing.T) {
	base := baselineWith(openPortOf("192.168.1.20", 80, "http"), openPortOf("192.168.1.20", 22, "ssh"))
	d := newDeepChecks(&fakeDeepScanner{}, base, &memDeepStore{})
	web := &fakeWeb{ok: true, res: nuclei.Result{Hits: []nuclei.Hit{hit("a", "Page A", "info", 80)}}}
	d.WithWebChecks(web)
	check, err := d.Run(context.Background(), "ws", "audit", "192.168.1.20")
	if err != nil {
		t.Fatal(err)
	}
	if len(web.targets) != 1 || web.targets[0].Port != 80 {
		t.Fatalf("targets = %+v, want only the web port", web.targets)
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
