package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	outside "llm-proxy/internal/core/netaudit/outsidelookup"
)

type fakeLookup struct {
	ports []int
	err   error
	asked []netip.Addr
}

func (f *fakeLookup) OpenPorts(_ context.Context, a netip.Addr) ([]int, error) {
	f.asked = append(f.asked, a)
	return f.ports, f.err
}

var (
	gateway    = netip.MustParseAddr("192.168.50.1")
	publicAddr = netip.MustParseAddr("203.0.113.7")
	lookedAt   = time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)
)

func exposureOf(on bool, l *fakeLookup) *Exposure {
	return NewExposure(func() bool { return on }, l, func() time.Time { return lookedAt })
}

// answering is a router that answers UPnP and states ext as its internet address; "" is one that states none, as when its answer could not be read.
func answering(ext string, maps ...RouterMapping) RouterReport {
	addr, _ := netip.ParseAddr(ext)
	return RouterReport{Answered: true, Gateway: true, HasService: true, ExternalAddr: addr, Mappings: maps}
}

func TestExposure_APortTheServiceSeesAndAMappingExplainsIsAFindingOnTheRouter(t *testing.T) {
	l := &fakeLookup{ports: []int{8080}}
	got := exposureOf(true, l).Findings(context.Background(), gateway, answering("203.0.113.7", mapping(8080, 80, "tcp", "192.168.50.77")))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	f := got[0]
	if f.Check != CheckInternetExposedPort || f.Device != "192.168.50.1" || f.Port != 8080 || f.Severity != SeverityMedium {
		t.Errorf("finding = %+v", f)
	}
	if !strings.Contains(f.Evidence, "UPnP") || !strings.Contains(f.Evidence, "8080") || !strings.Contains(f.Evidence, "a week") {
		t.Errorf("wording = %q", f.Evidence)
	}
	if len(l.asked) != 1 || l.asked[0] != publicAddr {
		t.Errorf("asked %v", l.asked)
	}
}

func TestExposure_APortNoMappingExplainsSaysSoWithoutNamingACause(t *testing.T) {
	got := exposureOf(true, &fakeLookup{ports: []int{1194}}).Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(got) != 1 || !strings.Contains(got[0].Evidence, "No UPnP mapping") {
		t.Fatalf("got %+v", got)
	}
	for _, cause := range []string{"DMZ", "double NAT", "VPN", "OpenVPN"} {
		if strings.Contains(got[0].Evidence, cause) {
			t.Errorf("the wording must not name a cause (%s): %q", cause, got[0].Evidence)
		}
	}
}

func TestExposure_ADisabledMappingExplainsNothing(t *testing.T) {
	m := mapping(8080, 80, "tcp", "192.168.50.77")
	m.Enabled = false
	got := exposureOf(true, &fakeLookup{ports: []int{8080}}).Findings(context.Background(), gateway, answering("203.0.113.7", m))
	if len(got) != 1 || strings.Contains(got[0].Evidence, "asked it to through UPnP") {
		t.Errorf("got %+v", got)
	}
}

func TestExposure_ASensitivePortIsHigh(t *testing.T) {
	got := exposureOf(true, &fakeLookup{ports: []int{22, 3389}}).Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(got) != 2 || got[0].Severity != SeverityHigh || got[1].Severity != SeverityHigh {
		t.Errorf("got %+v", got)
	}
}

// The public address identifies the household: it stays out of every finding, which goes to messaging apps.
func TestExposure_TheAddressIsNeverInAFinding(t *testing.T) {
	got := exposureOf(true, &fakeLookup{ports: []int{1194}}).Findings(context.Background(), gateway, answering("203.0.113.7"))
	for _, f := range got {
		if strings.Contains(f.Evidence, "203.0.113") || strings.Contains(f.Device, "203.0.113") {
			t.Errorf("the public address leaked: %+v", f)
		}
	}
}

func TestExposure_SwitchedOffAsksNobodyAndChangesNothing(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	e := exposureOf(false, l)
	if got := e.Findings(context.Background(), gateway, answering("203.0.113.7")); got != nil || len(l.asked) != 0 {
		t.Errorf("got %+v, asked %v", got, l.asked)
	}
	if s := e.Status(); s.Outcome != "" {
		t.Errorf("status = %+v", s)
	}
}

func TestExposure_WhenItCannotLookItSaysWhyAndSendsNothing(t *testing.T) {
	cases := map[string]struct {
		report RouterReport
		reason string
	}{
		"router says nothing":   {RouterReport{}, ReasonUPnPOff},
		"not a gateway":         {RouterReport{Answered: true}, ReasonUPnPOff},
		"no internet service":   {RouterReport{Answered: true, Gateway: true}, ReasonNoAddress},
		"address unreadable":    {answering("unreadable"), ReasonNoAddress},
		"address not given":     {answering(""), ReasonNoAddress},
		"another router before": {answering("192.168.0.137"), ReasonPrivate},
		"carrier grade nat":     {answering("100.64.1.1"), ReasonShared},
		"link local":            {answering("169.254.3.4"), ReasonLinkLocal},
		"loopback":              {answering("127.0.0.1"), ReasonNoAddress},
	}
	for name, c := range cases {
		l := &fakeLookup{ports: []int{1194}}
		e := exposureOf(true, l)
		if got := e.Findings(context.Background(), gateway, c.report); len(got) != 0 || len(l.asked) != 0 {
			t.Errorf("%s: got %+v, asked %v", name, got, l.asked)
		}
		if s := e.Status(); s.Outcome != OutcomeCannotLook || s.Reason != c.reason || !s.At.Equal(lookedAt) {
			t.Errorf("%s: status = %+v, want reason %s", name, s, c.reason)
		}
	}
}

func TestExposure_AFailedLookupIsFailedNeverNothingOpen(t *testing.T) {
	e := exposureOf(true, &fakeLookup{err: errors.New("blocked")})
	if got := e.Findings(context.Background(), gateway, answering("203.0.113.7")); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	if s := e.Status(); s.Outcome != OutcomeFailed || s.Reason != ReasonLookupFailed {
		t.Errorf("status = %+v", s)
	}
}

func TestExposure_ALookWithNothingOpenIsSaid(t *testing.T) {
	e := exposureOf(true, &fakeLookup{})
	if got := e.Findings(context.Background(), gateway, answering("203.0.113.7")); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	if s := e.Status(); s.Outcome != OutcomeLooked || s.Ports != 0 {
		t.Errorf("status = %+v", s)
	}
}

// Without this a night the service could not be reached would report every exposed port as fixed.
func TestExposure_CarryKeepsEarlierFindingsOnlyWhenTheLastAttemptDidNotLook(t *testing.T) {
	earlier := []Finding{
		{Device: "192.168.50.1", Port: 1194, Check: CheckInternetExposedPort, Severity: SeverityMedium, Evidence: "e"},
		{Device: "192.168.50.1", Check: CheckUPnPEnabled, Severity: SeverityLow, Evidence: "u"},
	}
	l := &fakeLookup{err: errors.New("down")}
	e := exposureOf(true, l)
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if got := e.Carry(earlier); len(got) != 1 || got[0].Check != CheckInternetExposedPort {
		t.Errorf("after a failure: %+v", got)
	}
	e.Findings(context.Background(), gateway, RouterReport{}) // UPnP switched off
	if got := e.Carry(earlier); len(got) != 1 {
		t.Errorf("when it cannot look: %+v", got)
	}
	l.err, l.ports = nil, []int{1194}
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if got := e.Carry(earlier); len(got) != 0 {
		t.Errorf("a real look replaces what was known: %+v", got)
	}
	if got := exposureOf(true, l).Carry(earlier); len(got) != 0 {
		t.Errorf("nothing was attempted yet: %+v", got)
	}
	off := exposureOf(true, &fakeLookup{err: errors.New("x")})
	off.Findings(context.Background(), gateway, answering("203.0.113.7"))
	off.enabled = func() bool { return false }
	if got := off.Carry(earlier); len(got) != 0 {
		t.Errorf("switched off: the findings go: %+v", got)
	}
}

// A rescan of one device runs the router check too: the service must not be asked again for the same address within hours.
func TestExposure_ASecondLookSoonAfterReusesTheAnswer(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	now := lookedAt
	e := NewExposure(func() bool { return true }, l, func() time.Time { return now })
	first := e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	now = now.Add(time.Hour)
	second := e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(l.asked) != 1 || len(first) != 1 || len(second) != 1 {
		t.Fatalf("asked %d times, findings %d and %d", len(l.asked), len(first), len(second))
	}
	if s := e.Status(); !s.At.Equal(lookedAt) {
		t.Errorf("the status keeps the time of the look that was really made: %v", s.At)
	}
	now = now.Add(exposureReuse)
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(l.asked) != 2 {
		t.Errorf("after the window the service is asked again: %d", len(l.asked))
	}
}

func TestExposure_AnotherAddressOrAFailureIsNeverReused(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	e := exposureOf(true, l)
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	e.Findings(context.Background(), gateway, answering("203.0.113.8"))
	if len(l.asked) != 2 {
		t.Errorf("a changed address is a new question: %d", len(l.asked))
	}
	f := &fakeLookup{err: errors.New("down")}
	e = exposureOf(true, f)
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(f.asked) != 2 {
		t.Errorf("a failure is tried again: %d", len(f.asked))
	}
}

func TestExposure_TheReusedAnswerIsComparedWithTheMappingsOfNow(t *testing.T) {
	l := &fakeLookup{ports: []int{8080}}
	e := exposureOf(true, l)
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	got := e.Findings(context.Background(), gateway, answering("203.0.113.7", mapping(8080, 80, "tcp", "192.168.50.77")))
	if len(l.asked) != 1 || len(got) != 1 || !strings.Contains(got[0].Evidence, "asked it to through UPnP") {
		t.Errorf("asked %d, got %+v", len(l.asked), got)
	}
}

func TestExposure_InternetAccessOffIsItsOwnReason(t *testing.T) {
	e := exposureOf(true, &fakeLookup{err: outside.ErrInternetOff})
	if got := e.Findings(context.Background(), gateway, answering("203.0.113.7")); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	if s := e.Status(); s.Outcome != OutcomeFailed || s.Reason != ReasonInternetOff {
		t.Errorf("status = %+v", s)
	}
}

// The router is asked to state its internet address (RouterProber.Probe's wantAddress) only while the lookup is on.
func TestExposure_TheRouterIsToldToStateItsAddressOnlyWhileTheLookupIsOn(t *testing.T) {
	if exposureOf(false, &fakeLookup{}).Wants() {
		t.Error("switched off: the router must not be asked for its address")
	}
	if !exposureOf(true, &fakeLookup{}).Wants() {
		t.Error("switched on: the router is asked")
	}
}

// A router that states an address but has no internet service to hold it is read as giving none.
func TestExposure_AnAddressWithoutAnInternetServiceIsNotUsed(t *testing.T) {
	l := &fakeLookup{ports: []int{1}}
	report := answering("203.0.113.7")
	report.HasService = false
	if got := exposureOf(true, l).Findings(context.Background(), gateway, report); len(got) != 0 || len(l.asked) != 0 {
		t.Errorf("got %+v, asked %v", got, l.asked)
	}
}

// withEntered is the check with an address the operator typed, for when the router gives none.
func withEntered(l *fakeLookup, entered string) *Exposure {
	return exposureOf(true, l).WithEnteredAddress(func() string { return entered })
}

func TestExposure_AnEnteredAddressIsUsedWhenTheRouterGivesNone(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	e := withEntered(l, "203.0.113.7")
	got := e.Findings(context.Background(), gateway, RouterReport{})
	if len(got) != 1 || got[0].Port != 1194 || len(l.asked) != 1 || l.asked[0] != publicAddr {
		t.Fatalf("got %+v asked %v", got, l.asked)
	}
	if s := e.Status(); s.Outcome != OutcomeLooked || s.Source != SourceEntered || s.Differs {
		t.Errorf("status = %+v", s)
	}
}

func TestExposure_AnEnteredAddressIsUsedWhenTheRoutersIsNotPublic(t *testing.T) {
	for _, ext := range []string{"192.168.0.137", "100.64.3.2", "169.254.1.1", "unreadable"} {
		l := &fakeLookup{ports: []int{1194}}
		e := withEntered(l, "203.0.113.7")
		if got := e.Findings(context.Background(), gateway, answering(ext)); len(got) != 1 || e.Status().Source != SourceEntered {
			t.Errorf("router said %q: got %+v status %+v", ext, got, e.Status())
		}
	}
}

func TestExposure_ThePublicAddressTheRouterStatesWinsOverAnEnteredOne(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	e := withEntered(l, "198.51.100.9")
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if len(l.asked) != 1 || l.asked[0] != publicAddr {
		t.Errorf("asked %v", l.asked)
	}
	if s := e.Status(); s.Source != SourceRouter || !s.Differs {
		t.Errorf("a different entered address is flagged: %+v", s)
	}
}

func TestExposure_ARouterAddressThatMatchesTheEnteredOneIsNotFlagged(t *testing.T) {
	e := withEntered(&fakeLookup{}, "203.0.113.7")
	e.Findings(context.Background(), gateway, answering("203.0.113.7"))
	if s := e.Status(); s.Source != SourceRouter || s.Differs {
		t.Errorf("status = %+v", s)
	}
}

func TestExposure_AnEnteredAddressThatIsNotPublicIsIgnored(t *testing.T) {
	l := &fakeLookup{ports: []int{1194}}
	e := withEntered(l, "192.168.0.9")
	if got := e.Findings(context.Background(), gateway, RouterReport{}); len(got) != 0 || len(l.asked) != 0 {
		t.Fatalf("nothing may be sent: %+v %v", got, l.asked)
	}
	if s := e.Status(); s.Outcome != OutcomeCannotLook || s.Reason != ReasonUPnPOff {
		t.Errorf("status = %+v", s)
	}
}

func TestExposure_ALookAtAnEnteredAddressThatFailsKeepsTheSource(t *testing.T) {
	e := withEntered(&fakeLookup{err: errors.New("down")}, "203.0.113.7")
	e.Findings(context.Background(), gateway, RouterReport{})
	if s := e.Status(); s.Outcome != OutcomeFailed || s.Source != SourceEntered {
		t.Errorf("status = %+v", s)
	}
}
