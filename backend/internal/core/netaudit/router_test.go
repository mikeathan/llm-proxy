package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

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
