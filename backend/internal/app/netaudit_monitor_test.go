package app

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/platform/auth"
	"llm-proxy/models"
)

var monitorNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)

func TestAuditReading_TheLatestAuditedDayIsTheLastGoodAndNoAuditMeansOff(t *testing.T) {
	series := func(days ...models.NetworkDay) func(int) ([]models.NetworkDay, error) {
		return func(int) ([]models.NetworkDay, error) { return days, nil }
	}
	got := auditReading(series(models.NetworkDay{Date: "2026-11-01", Audited: true}, models.NetworkDay{Date: "2026-11-03"}))
	want := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local) // the end of the audited day
	if !got.On || !got.LastGood.Equal(want) || got.Limit != auditStallLimit {
		t.Fatalf("reading = %+v, want on, last good %v", got, want)
	}
	if r := auditReading(series(models.NetworkDay{Date: "2026-11-03"})); r.On {
		t.Fatalf("no audit in the history must read as not switched on: %+v", r)
	}
	failing := func(int) ([]models.NetworkDay, error) { return nil, errors.New("disk") }
	if r := auditReading(failing); r.On {
		t.Fatalf("an unreadable history must not raise a stall: %+v", r)
	}
}

func TestDecoyReading_HealthyOnlyWhileListeningWithoutAProblem(t *testing.T) {
	if r := decoyReading(true, decoy.Status{Listening: []int{2222}}, monitorNow); !r.LastGood.Equal(monitorNow) {
		t.Fatalf("listening decoys are healthy now: %+v", r)
	}
	for _, st := range []decoy.Status{{Listening: []int{}}, {Listening: []int{2222}, Problem: "port in use"}} {
		if r := decoyReading(true, st, monitorNow); !r.LastGood.IsZero() {
			t.Fatalf("status %+v must not count as working: %+v", st, r)
		}
	}
}

func TestRouterLogReadings_TheSocketAndTheQuietAreWatchedApart(t *testing.T) {
	heard := monitorNow.Add(-2 * time.Hour)
	socket, quiet := routerLogReadings(true, routerlog.Status{Listening: true, LastHeard: heard}, monitorNow)
	if !socket.LastGood.Equal(monitorNow) || !quiet.LastGood.Equal(heard) {
		t.Fatalf("socket = %+v, quiet = %+v", socket, quiet)
	}
	socket, _ = routerLogReadings(true, routerlog.Status{Listening: false, Problem: "port in use"}, monitorNow)
	if !socket.LastGood.IsZero() {
		t.Fatalf("a socket that is not open is not working: %+v", socket)
	}
}

func TestPresenceAndDNSReadings_TheLimitFollowsTheInterval(t *testing.T) {
	p := presenceReading(netaudit.PresenceSettings{Enabled: true, Every: time.Hour}, netaudit.PresenceStatus{LastLook: monitorNow})
	if p.Limit != 3*time.Hour || !p.LastGood.Equal(monitorNow) {
		t.Fatalf("presence = %+v", p)
	}
	d := dnsReading(netaudit.DNSSettings{Enabled: true, Every: time.Minute, Names: []string{"example.org"}}, netaudit.DNSStatus{LastRun: monitorNow, Problem: "refused"})
	if d.Limit != minMonitorLimit || !d.LastGood.IsZero() {
		t.Fatalf("a refused DNS cycle is not a success, and the limit has a floor: %+v", d)
	}
	if r := dnsReading(netaudit.DNSSettings{Enabled: true}, netaudit.DNSStatus{}); r.On {
		t.Fatalf("a DNS check with no names does nothing, so it is not watched: %+v", r)
	}
}

func TestPrivilegeReader_OnlyLinuxIsRead(t *testing.T) {
	if got := privilegeReader("darwin").Check(); got.Checked {
		t.Fatalf("macOS must read as not checked: %+v", got)
	}
	if r := privilegeReader("linux"); r.Passwd != "/etc/passwd" || len(r.SetIDRoots) != 3 {
		t.Fatalf("linux reader = %+v", r)
	}
}

func TestSignInAlertNames_TheSignInAndTheAlertsAgree(t *testing.T) {
	if string(auth.SignInLocked) != netaudit.SignInLockedOut || string(auth.SignInNewAddress) != netaudit.SignInFromNewAddress {
		t.Fatal("the sign-in's event names must be the ones the Network alerts know")
	}
}
