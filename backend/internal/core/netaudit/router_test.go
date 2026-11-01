package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/upnp"
	"llm-proxy/models"
)

type fakeProber struct {
	report upnp.Report
	err    error
	asked  []netip.Addr
}

func (f *fakeProber) Probe(_ context.Context, gw netip.Addr) (upnp.Report, error) {
	f.asked = append(f.asked, gw)
	return f.report, f.err
}

func routerCheck(p *fakeProber, gateway string, allowed error) *RouterCheck {
	return NewRouterCheck(p, func() (netip.Addr, error) {
		if gateway == "" {
			return netip.Addr{}, nil
		}
		return netip.MustParseAddr(gateway), nil
	}, func(netip.Addr) error { return allowed })
}

func TestRouterCheck_UPnPOnTheRouterIsAFinding(t *testing.T) {
	p := &fakeProber{report: upnp.Report{Answered: true, Gateway: true, Service: upnp.ServiceWANIP}}
	got, err := routerCheck(p, "192.168.50.1", nil).Findings(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	f := got[0]
	if f.Check != CheckUPnPEnabled || f.Device != "192.168.50.1" || f.Severity != SeverityLow || f.Port != 0 {
		t.Errorf("finding = %+v", f)
	}
	if f.Key() != "192.168.50.1||0|upnp_enabled" {
		t.Errorf("key %q", f.Key())
	}
	if p.asked[0] != netip.MustParseAddr("192.168.50.1") {
		t.Errorf("asked %v", p.asked)
	}
}

func TestRouterCheck_NoAnswerAndNotAGatewayAreNotFindings(t *testing.T) {
	for _, r := range []upnp.Report{{}, {Answered: true}} {
		got, err := routerCheck(&fakeProber{report: r}, "192.168.50.1", nil).Findings(context.Background())
		if err != nil || len(got) != 0 {
			t.Errorf("report %+v gave %+v, %v", r, got, err)
		}
	}
}

func TestRouterCheck_OnlyTheRouterInsideTheScopeIsAsked(t *testing.T) {
	p := &fakeProber{}
	if _, err := routerCheck(p, "203.0.113.1", errors.New("out of scope")).Findings(context.Background()); err == nil {
		t.Error("a router outside the scan scope is refused, not probed")
	}
	if len(p.asked) != 0 {
		t.Errorf("the probe must not run: %v", p.asked)
	}
}

func TestRouterCheck_TroubleIsAnErrorNeverAnEmptyList(t *testing.T) {
	if _, err := routerCheck(&fakeProber{err: errors.New("blocked")}, "192.168.50.1", nil).Findings(context.Background()); err == nil {
		t.Error("a refused probe is 'could not check'")
	}
	if _, err := routerCheck(&fakeProber{}, "", nil).Findings(context.Background()); !errors.Is(err, ErrNoRouter) {
		t.Errorf("an unknown router is said to be unknown: %v", err)
	}
}

type fakeRouterFindings struct {
	findings []Finding
	err      error
}

func (f fakeRouterFindings) Findings(context.Context) ([]Finding, error) { return f.findings, f.err }

func TestRunReporter_TheRouterCheckJoinsTheReport(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	up := Finding{Device: "10.0.0.1", Check: CheckUPnPEnabled, Severity: SeverityLow, Evidence: "x"}
	r := reporter(store, reportTime).WithRouter(fakeRouterFindings{findings: []Finding{up}})
	got := r.Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
	if !strings.Contains(got, "UPnP") {
		t.Errorf("report:\n%s", got)
	}
	// A router this scan did not look at is left out.
	store2 := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store2, reportTime).WithRouter(fakeRouterFindings{findings: []Finding{up}}).Augment(scannedHosts(hostOf("10.0.0.9", true, false, 80)), "ws", "nightly", "ok")
	for _, f := range store2.data["ws/nightly"].Findings {
		if f.Check == CheckUPnPEnabled {
			t.Errorf("not scanned, not reported: %+v", f)
		}
	}
}

func TestRunReporter_RouterTroubleIsStated(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime).WithRouter(fakeRouterFindings{err: errors.New("blocked")})
	got := r.Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
	if !strings.Contains(got, "could not be checked") {
		t.Errorf("report:\n%s", got)
	}
}

func mapping(ext, in int, proto, client string) upnp.Mapping {
	return upnp.Mapping{ExternalPort: ext, InternalPort: in, Protocol: proto, Client: netip.MustParseAddr(client), Enabled: true}
}

func TestRouterCheck_AForwardedPortIsAFindingOnTheDeviceItGoesTo(t *testing.T) {
	p := &fakeProber{report: upnp.Report{Answered: true, Gateway: true, Service: upnp.ServiceWANIP, Mappings: []upnp.Mapping{
		mapping(8080, 8080, "tcp", "192.168.50.77"), mapping(2222, 22, "tcp", "192.168.50.80"), mapping(9, 9, "udp", "192.168.50.77"),
	}}}
	got, err := routerCheck(p, "192.168.50.1", nil).Findings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Finding{}
	for _, f := range got {
		by[f.Key()] = f
	}
	web, ssh := by["192.168.50.77|tcp|8080|upnp_forward"], by["192.168.50.80|tcp|22|upnp_forward"]
	if web.Severity != SeverityMedium || ssh.Severity != SeverityHigh {
		t.Errorf("a web port is medium and an SSH port high: %+v / %+v", web, ssh)
	}
	if !strings.Contains(web.Evidence, "8080") || !strings.Contains(ssh.Evidence, "2222") {
		t.Errorf("evidence names the ports: %q / %q", web.Evidence, ssh.Evidence)
	}
	if _, ok := by["192.168.50.77|udp|9|upnp_forward"]; !ok {
		t.Error("UDP forwards count too")
	}
	if _, ok := by["192.168.50.1||0|upnp_enabled"]; !ok {
		t.Error("UPnP being on is still reported")
	}
}

func TestRouterCheck_ADisabledOrOutsideForwardIsHandledHonestly(t *testing.T) {
	off := mapping(80, 80, "tcp", "192.168.50.77")
	off.Enabled = false
	out := mapping(80, 80, "tcp", "203.0.113.9")
	p := &fakeProber{report: upnp.Report{Answered: true, Gateway: true, Mappings: []upnp.Mapping{off, out}}}
	got, _ := routerCheck(p, "192.168.50.1", nil).Findings(context.Background())
	for _, f := range got {
		if f.Device == "192.168.50.77" {
			t.Errorf("a disabled mapping forwards nothing: %+v", f)
		}
	}
	var outside *Finding
	for i := range got {
		if got[i].Device == "203.0.113.9" {
			outside = &got[i]
		}
	}
	if outside == nil || outside.Severity != SeverityHigh || !strings.Contains(outside.Evidence, "outside") {
		t.Errorf("a forward to an address outside the network is high: %+v", outside)
	}
}

func TestRouterCheck_NoTextFromTheRouterReachesAFinding(t *testing.T) {
	p := &fakeProber{report: upnp.Report{Answered: true, Gateway: true, Mappings: []upnp.Mapping{mapping(8080, 8080, "tcp", "192.168.50.77")}}}
	got, _ := routerCheck(p, "192.168.50.1", nil).Findings(context.Background())
	for _, f := range got {
		if strings.ContainsAny(f.Evidence, "<>") {
			t.Errorf("evidence = %q", f.Evidence)
		}
	}
}

func TestRouterCheck_ListingTroubleKeepsWhatWasFoundAndSaysSo(t *testing.T) {
	p := &fakeProber{report: upnp.Report{Answered: true, Gateway: true, Mappings: []upnp.Mapping{mapping(80, 80, "tcp", "192.168.50.77")}, MappingsErr: "boom"}}
	got, err := routerCheck(p, "192.168.50.1", nil).Findings(context.Background())
	if err == nil || len(got) != 2 {
		t.Errorf("findings %d, err %v: the findings stand and the trouble is stated", len(got), err)
	}
}

func TestRouterCheck_TheOutsideLookupJoinsTheRoutersFindings(t *testing.T) {
	p := &fakeProber{report: answering("203.0.113.7", mapping(8080, 80, "tcp", "192.168.50.77"))}
	e := exposureOf(true, &fakeLookup{ports: []int{8080}})
	got, err := routerCheck(p, "192.168.50.1", nil).WithExposure(e).Findings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, f := range got {
		kinds = append(kinds, f.Check)
	}
	if !strings.Contains(strings.Join(kinds, ","), CheckInternetExposedPort) || !strings.Contains(strings.Join(kinds, ","), CheckUPnPEnabled) {
		t.Errorf("findings = %v", kinds)
	}
}

// A router with UPnP off says nothing: the outside lookup must still be told, so its status says why it could not look.
func TestRouterCheck_AQuietRouterStillRecordsWhyTheOutsideLookupCouldNotLook(t *testing.T) {
	e := exposureOf(true, &fakeLookup{ports: []int{1194}})
	got, err := routerCheck(&fakeProber{}, "192.168.50.1", nil).WithExposure(e).Findings(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if s := e.Status(); s.Outcome != OutcomeCannotLook || s.Reason != ReasonUPnPOff {
		t.Errorf("status = %+v", s)
	}
}

func exposedBefore() models.ScanBaseline {
	return models.ScanBaseline{ScannedAt: reportTime.Add(-24 * time.Hour), Findings: []Finding{
		{Device: "10.0.0.1", Port: 1194, Check: CheckInternetExposedPort, Severity: SeverityMedium, Evidence: "e"},
	}}
}

func TestRunReporter_AnExposureIsNotResolvedByANightTheServiceCouldNotBeAsked(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{"ws/nightly": exposedBefore()}}
	check := NewRouterCheck(&fakeProber{report: answering("203.0.113.7")}, func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil },
		func(netip.Addr) error { return nil }).WithExposure(exposureOf(true, &fakeLookup{err: errors.New("down")}))
	r := reporter(store, reportTime).WithRouter(check)
	r.Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
	got := store.data["ws/nightly"]
	if len(got.Resolved) != 0 {
		t.Errorf("resolved = %+v", got.Resolved)
	}
	var kept bool
	for _, f := range got.Findings {
		kept = kept || f.Check == CheckInternetExposedPort
	}
	if !kept {
		t.Errorf("the exposure must stay in the baseline: %+v", got.Findings)
	}
}

func TestRunReporter_AnExposureThatIsReallyGoneIsResolved(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{"ws/nightly": exposedBefore()}}
	check := NewRouterCheck(&fakeProber{report: answering("203.0.113.7")}, func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil },
		func(netip.Addr) error { return nil }).WithExposure(exposureOf(true, &fakeLookup{}))
	reporter(store, reportTime).WithRouter(check).Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
	if got := store.data["ws/nightly"]; len(got.Resolved) != 1 || got.Resolved[0].Check != CheckInternetExposedPort {
		t.Errorf("resolved = %+v", got.Resolved)
	}
}

// Whatever stops the router check before it can look must leave the earlier exposure standing, from a fresh program and from
// one that had looked before.
func TestRunReporter_AnExposureSurvivesEveryWayTheRouterCheckCanFailBeforeLooking(t *testing.T) {
	gwOf := func(addr string, err error) func() (netip.Addr, error) {
		return func() (netip.Addr, error) {
			if addr == "" {
				return netip.Addr{}, err
			}
			return netip.MustParseAddr(addr), err
		}
	}
	allow := func(netip.Addr) error { return nil }
	cases := map[string]*RouterCheck{
		"probe fails":       NewRouterCheck(&fakeProber{err: errors.New("blocked")}, gwOf("10.0.0.1", nil), allow),
		"gateway unknown":   NewRouterCheck(&fakeProber{}, gwOf("", nil), allow),
		"gateway unread":    NewRouterCheck(&fakeProber{}, gwOf("", errors.New("no table")), allow),
		"outside the scope": NewRouterCheck(&fakeProber{}, gwOf("10.0.0.1", nil), func(netip.Addr) error { return errors.New("no") }),
	}
	for name, check := range cases {
		for _, warm := range []bool{false, true} {
			e := exposureOf(true, &fakeLookup{ports: []int{1194}})
			if warm { // an earlier night's look was made in this very program
				e.Findings(context.Background(), gateway, answering("203.0.113.7"))
			}
			store := &memBaselines{data: map[string]models.ScanBaseline{"ws/nightly": exposedBefore()}}
			reporter(store, reportTime).WithRouter(check.WithExposure(e)).Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
			got := store.data["ws/nightly"]
			if len(got.Resolved) != 0 || countCheck(got.Findings, CheckInternetExposedPort) != 1 {
				t.Errorf("%s (warm=%v): resolved %+v, findings %+v", name, warm, got.Resolved, got.Findings)
			}
		}
	}
}

func countCheck(findings []Finding, check string) int {
	n := 0
	for _, f := range findings {
		if f.Check == check {
			n++
		}
	}
	return n
}

// A look made, then a night the router said nothing, then a night it answered again within the reuse window: the reused answer
// is the finding, and the earlier one is not carried on top of it.
func TestRunReporter_AReusedLookIsNotCarriedOnTopOfItself(t *testing.T) {
	prober := &fakeProber{report: answering("203.0.113.7")}
	e := exposureOf(true, &fakeLookup{ports: []int{1194}})
	check := NewRouterCheck(prober, func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }, func(netip.Addr) error { return nil }).WithExposure(e)
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	run := func() {
		reporter(store, reportTime).WithRouter(check).Augment(scannedHosts(hostOf("10.0.0.1", true, false, 80)), "ws", "nightly", "ok")
	}
	run() // looked
	prober.report = upnp.Report{}
	run() // the router says nothing: carried
	prober.report = answering("203.0.113.7")
	run() // answers again, the cached look is reused
	if n := countCheck(store.data["ws/nightly"].Findings, CheckInternetExposedPort); n != 1 {
		t.Errorf("one exposure, not %d: %+v", n, store.data["ws/nightly"].Findings)
	}
	if s := e.Status(); s.Outcome != OutcomeLooked {
		t.Errorf("a reused look is a look that was made: %+v", s)
	}
}
