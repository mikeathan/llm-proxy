package netaudit

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/models"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
)

type routerRig struct {
	w     *RouterWatch
	now   time.Time
	sent  []string
	on    bool
	fails bool
}

func newRouterRig() *routerRig {
	r := &routerRig{now: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), on: true}
	r.w = NewRouterWatch(RouterWatchDeps{
		Enabled: func() bool { return r.on },
		Notify: func(_ context.Context, m string) error {
			if r.fails {
				return errors.New("no connector")
			}
			r.sent = append(r.sent, m)
			return nil
		},
		Now: func() time.Time { return r.now },
	})
	return r
}

func (r *routerRig) add(kind routerlog.WatchedKind, n int, every time.Duration) {
	for i := 0; i < n; i++ {
		r.w.Add(routerlog.Watched{Kind: kind, Reason: 3}, r.now)
		r.now = r.now.Add(every)
	}
}

func TestRouterWatch_FiveFailedSignInsInTenMinutesAreToldOnce(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Minute)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "sign-in") || !strings.Contains(r.sent[0], "5") {
		t.Fatalf("sent = %q", r.sent)
	}
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Errorf("the same burst was told twice: %q", r.sent)
	}
}

func TestRouterWatch_FailuresSpreadOverMoreThanTheWindowAreNotABurst(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, 20, 3*time.Minute) // one every three minutes: four in ten
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_TwentyDisconnectsInAMinuteAreToldAndAFewAreNot(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.Deauth, routerDeauthBurst-1, time.Second) // the rate of a phone toggling its Wi-Fi, and more
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Fatalf("nineteen is not a flood: %q", r.sent)
	}
	r.add(routerlog.Deauth, 1, time.Second)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "Wi-Fi") {
		t.Fatalf("sent = %q", r.sent)
	}
}

func TestRouterWatch_ASecondBurstWithinTheGapWaitsAndIsToldAfterIt(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %q", r.sent)
	}
	r.now = r.now.Add(routerAlertGap)
	r.w.Tick(context.Background())
	if len(r.sent) != 2 {
		t.Errorf("the held burst was never told: %q", r.sent)
	}
}

func TestRouterWatch_AMessageThatCouldNotBeSentIsTriedAgain(t *testing.T) {
	r := newRouterRig()
	r.fails = true
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	r.fails = false
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_WhileSwitchedOffNothingIsKeptOrSent(t *testing.T) {
	r := newRouterRig()
	r.on = false
	r.add(routerlog.SignInFailed, routerSignInFailures*2, time.Second)
	r.on = true
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_ItsMemoryStaysTheSizeOfTheBurstWhateverIsSent(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.Deauth, 100000, time.Millisecond)
	r.add(routerlog.SignInFailed, 100000, time.Millisecond)
	if n := len(r.w.deauths) + len(r.w.fails); n > routerDeauthBurst+routerSignInFailures {
		t.Errorf("%d times kept", n)
	}
}

func (r *routerRig) deauthFor(mac string, n int) {
	for i := 0; i < n; i++ {
		r.w.Add(routerlog.Watched{Kind: routerlog.Deauth, Reason: 3, MAC: mac}, r.now)
		r.now = r.now.Add(time.Minute) // slow enough to be no burst
	}
}

func TestRouterWatch_SnapshotCountsEachDevicesDisconnectsAndTheBurstsOfTheLastDay(t *testing.T) {
	r := newRouterRig()
	r.deauthFor("02:aa:bb:00:00:01", 7)
	r.deauthFor("02:aa:bb:00:00:02", 2)
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	got := r.w.Snapshot(r.now)
	if got.Devices["02:aa:bb:00:00:01"] != 7 || got.Devices["02:aa:bb:00:00:02"] != 2 || got.SignInBursts != 1 || got.DeauthBursts != 0 {
		t.Fatalf("snapshot = %+v", got)
	}
	later := r.now.Add(25 * time.Hour)
	if got := r.w.Snapshot(later); len(got.Devices) != 0 || got.SignInBursts != 0 {
		t.Errorf("a day later: %+v", got)
	}
}

func TestRouterWatch_TheDeviceTableAndTheBurstListStayBounded(t *testing.T) {
	r := newRouterRig()
	for i := 0; i < routerMaxDevices*3; i++ {
		r.w.Add(routerlog.Watched{Kind: routerlog.Deauth, Reason: 3, MAC: fmt.Sprintf("02:00:00:00:%02x:%02x", i/256, i%256)}, r.now)
	}
	for i := 0; i < routerMaxBursts*5; i++ {
		r.w.Add(routerlog.Watched{Kind: routerlog.Deauth, Reason: 3, MAC: "02:aa:bb:00:00:01"}, r.now)
	}
	got := r.w.Snapshot(r.now)
	if len(got.Devices) > routerMaxDevices || got.DeauthBursts > routerMaxBursts {
		t.Errorf("devices %d, bursts %d", len(got.Devices), got.DeauthBursts)
	}
}

func TestRouterWatch_SnapshotIsEmptyWhileSwitchedOff(t *testing.T) {
	r := newRouterRig()
	r.deauthFor("02:aa:bb:00:00:01", 3)
	r.on = false
	if got := r.w.Snapshot(r.now); len(got.Devices) != 0 {
		t.Errorf("snapshot = %+v", got)
	}
}

func TestRouterLogFindings_NameTheRouterForBurstsAndADeviceByItsAddressForManyDisconnects(t *testing.T) {
	inv := models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "02:aa:bb:00:00:01", IP: "192.168.50.20", Name: "Mike's phone"}, {MAC: "02:aa:bb:00:00:02", IP: "192.168.50.21"}}}
	snap := RouterSnapshot{SignInBursts: 2, DeauthBursts: 1, Devices: map[string]int{"02:aa:bb:00:00:01": routerDeviceDeauths, "02:aa:bb:00:00:02": routerDeviceDeauths - 1, "02:aa:bb:00:00:99": 5000}}
	got := RouterLogFindings(snap, "192.168.50.1", inv)
	byCheck := map[string]Finding{}
	for _, f := range got {
		byCheck[f.Check+"|"+f.Device] = f
	}
	if len(got) != 3 {
		t.Fatalf("findings = %+v", got)
	}
	if f := byCheck[CheckRouterSignInFailures+"|192.168.50.1"]; f.Severity != SeverityMedium || !strings.Contains(f.Evidence, "2") {
		t.Errorf("sign-in finding = %+v", f)
	}
	if _, ok := byCheck[CheckWifiDisconnectBurst+"|192.168.50.1"]; !ok {
		t.Error("no disconnect burst finding")
	}
	f, ok := byCheck[CheckWifiDeviceDisconnects+"|192.168.50.20"]
	if !ok || strings.Contains(f.Evidence, "Mike") || !strings.Contains(f.Evidence, "100") {
		t.Errorf("device finding = %+v", f)
	}
}

func TestRouterLogFindings_NothingWithoutTheRoutersAddressAndNothingBelowTheLimits(t *testing.T) {
	if got := RouterLogFindings(RouterSnapshot{SignInBursts: 3, DeauthBursts: 3}, "", models.DeviceInventory{}); len(got) != 0 {
		t.Errorf("findings = %+v", got)
	}
	if got := RouterLogFindings(RouterSnapshot{}, "192.168.50.1", models.DeviceInventory{}); len(got) != 0 {
		t.Errorf("findings = %+v", got)
	}
}
