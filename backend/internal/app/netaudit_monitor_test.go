package app

import (
	"errors"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/platform/auth"
	"llm-proxy/internal/platform/hostwatch"
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
	d := dnsReading(stacksdk.ModuleStatus{Enabled: true, Every: time.Minute, LastRun: monitorNow, Problem: "refused"})
	if d.Limit != minMonitorLimit || !d.LastGood.IsZero() {
		t.Fatalf("a refused DNS cycle is not a success, and the limit has a floor: %+v", d)
	}
	if r := dnsReading(stacksdk.ModuleStatus{}); r.On {
		t.Fatalf("a DNS check that is off or has nothing to run is not watched: %+v", r)
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

func TestAlertDeliveryReading_StallsOnlyWhileTheNewestSendsFailed(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return now.Add(time.Duration(-h) * time.Hour) }
	if r := alertDeliveryReading(nil, now); r.On {
		t.Error("no alert yet: nothing to watch")
	}
	sent := []models.NetworkAlert{{At: at(30), Delivery: models.NetworkAlertSent}}
	if r := alertDeliveryReading(sent, now); !r.On || !r.LastGood.Equal(now) {
		t.Errorf("the newest went out: healthy now, got %+v", r)
	}
	none := []models.NetworkAlert{{At: at(1), Delivery: models.NetworkAlertNoConnector}}
	if r := alertDeliveryReading(none, now); !r.LastGood.Equal(now) {
		t.Error("no connector is its own finding (alert_no_channel), not a stopped monitor")
	}
	failing := []models.NetworkAlert{ // newest first
		{At: at(1), Delivery: models.NetworkAlertFailed},
		{At: at(3), Delivery: models.NetworkAlertFailed},
		{At: at(30), Delivery: models.NetworkAlertSent},
	}
	if r := alertDeliveryReading(failing, now); !r.LastGood.Equal(at(3)) || r.Limit != alertDeliveryLimit {
		t.Errorf("failing since the oldest failure of the streak, got %+v", r)
	}
}

func TestPhoneHomeReading_OnWhileSwitchedOnAndTheLastDNSLineIsTheLastGood(t *testing.T) {
	last := monitorNow.Add(-3 * time.Hour)
	r := phoneHomeReading(true, routerlog.Status{QueriesLast: last})
	if !r.On || !r.LastGood.Equal(last) || r.Limit != phoneHomeStallLimit {
		t.Fatalf("reading = %+v", r)
	}
	if r := phoneHomeReading(false, routerlog.Status{QueriesLast: last}); r.On {
		t.Fatalf("a watch that is off is not watched: %+v", r)
	}
	// Nothing ever received: the limit starts when the watch is first seen on, so it says so after a day, not at once.
	if r := phoneHomeReading(true, routerlog.Status{}); !r.On || !r.LastGood.IsZero() {
		t.Fatalf("reading = %+v", r)
	}
}

// macOS ships sshd_config whether or not Remote Login is on, so there the file's presence must not be read as an SSH server being installed.
func TestHardenerFor_OnlyLinuxJudgesTheSSHServerSettings(t *testing.T) {
	base := hostwatch.Hardener{SSHDir: "/etc/ssh", SysDir: "/proc/sys"}
	if got := hardenerFor("linux", base); got.SSHDir != "/etc/ssh" {
		t.Errorf("linux keeps it: %+v", got)
	}
	for _, goos := range []string{"darwin", "freebsd", "windows"} {
		if got := hardenerFor(goos, base); got.SSHDir != "" || got.SysDir != "/proc/sys" {
			t.Errorf("%s: only the SSH folder is dropped: %+v", goos, got)
		}
	}
}

func TestDNSTallies_WordsACheckThatRanAndSaysNothingForOneThatDidNot(t *testing.T) {
	got := dnsTallies(stacksdk.ModuleStatus{Recent: stacksdk.Tally{Done: 24, Flagged: 1, Unclear: 2}})
	if len(got) != 1 || got[0].Line() != "Router DNS check, last 24 h: 24 comparisons, 1 differed, 2 could not be compared." {
		t.Errorf("tally = %+v", got)
	}
	if none := dnsTallies(stacksdk.ModuleStatus{}); len(none) != 0 {
		t.Errorf("a check that did nothing in the period adds no line: %+v", none)
	}
}
