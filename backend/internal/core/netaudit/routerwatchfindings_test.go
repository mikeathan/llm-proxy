package netaudit

import (
	"net/netip"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeRouterLog struct{ snap RouterSnapshot }

func (f fakeRouterLog) Snapshot(time.Time) RouterSnapshot { return f.snap }
func (fakeRouterLog) Router() string                      { return "192.168.50.1" }

func TestRunReporter_RouterLogFindingsAreAddedOnlyForAddressesThisScanLookedAt(t *testing.T) {
	r := &RunReporter{now: time.Now}
	r.WithRouterLog(fakeRouterLog{snap: RouterSnapshot{SignInBursts: 1}})
	scanned := Result{Hosts: []Host{{Addr: netip.MustParseAddr("192.168.50.1")}}}
	r.addRouterLog(&scanned, models.DeviceInventory{})
	if len(scanned.Findings) != 1 || scanned.Findings[0].Check != CheckRouterSignInFailures {
		t.Errorf("findings = %+v", scanned.Findings)
	}
	other := Result{Hosts: []Host{{Addr: netip.MustParseAddr("192.168.50.9")}}}
	r.addRouterLog(&other, models.DeviceInventory{})
	if len(other.Findings) != 0 {
		t.Errorf("a router this scan did not look at has a finding: %+v", other.Findings)
	}
	(&RunReporter{now: time.Now}).addRouterLog(&other, models.DeviceInventory{}) // without a source: nothing, no panic
}
