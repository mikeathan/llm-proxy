package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/wifilook"
	"llm-proxy/models"
)

var wifiNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

const (
	routerBand = "aa:bb:cc:00:00:01"
	meshBand   = "aa:bb:cc:00:00:02"
	twin       = "de:ad:be:ef:00:09"
	twin2      = "de:ad:be:ef:00:0a"
)

func ap(ssid, bssid string, sec wifilook.Security) wifilook.AP {
	return wifilook.AP{SSID: ssid, BSSID: bssid, Signal: 50, Security: sec}
}

type wifiRig struct {
	w     *WifiWatch
	cfg   *WifiConfig
	store *memConfig
	aps   []wifilook.AP
	err   error
	looks int
	sent  []string
	errs  []error
	now   time.Time
	fail  bool
}

func newWifiRig(t *testing.T) *wifiRig {
	t.Helper()
	r := &wifiRig{store: &memConfig{}, now: wifiNow}
	r.cfg = NewWifiConfig(r.store)
	if err := r.cfg.Set(true); err != nil {
		t.Fatal(err)
	}
	r.w = NewWifiWatch(WifiWatchDeps{
		List:   func(context.Context) ([]wifilook.AP, error) { r.looks++; return r.aps, r.err },
		Config: r.cfg,
		Notify: func(_ context.Context, m string) error {
			if r.fail {
				return errors.New("no connector")
			}
			r.sent = append(r.sent, m)
			return nil
		},
		OnError: func(err error) { r.errs = append(r.errs, err) },
		Now:     func() time.Time { return r.now },
	})
	return r
}

// confirmed sets up the operator's network "home" with the router's and the mesh node's addresses confirmed.
func (r *wifiRig) confirmed(t *testing.T) {
	t.Helper()
	if err := r.cfg.AddNetwork("home"); err != nil {
		t.Fatal(err)
	}
	r.aps = []wifilook.AP{ap("home", routerBand, wifilook.WPA2), ap("home", meshBand, wifilook.WPA2)}
	r.w.Tick(context.Background())
	if err := r.w.Confirm("home", []string{routerBand, meshBand}); err != nil {
		t.Fatal(err)
	}
}

func TestWifiWatch_AnUnconfirmedNameIsNotJudgedYet(t *testing.T) {
	r := newWifiRig(t)
	if err := r.cfg.AddNetwork("home"); err != nil {
		t.Fatal(err)
	}
	r.aps = []wifilook.AP{ap("home", routerBand, wifilook.WPA2), ap("home", twin, wifilook.Open)}
	r.w.Tick(context.Background())
	if len(r.sent) != 0 || len(r.w.Snapshot(r.now).Lookalikes) != 0 {
		t.Fatalf("sent = %q: until the operator says which addresses are theirs, every address would look like a copy", r.sent)
	}
	if v := r.w.View(); len(v.Networks) != 1 || len(v.Networks[0].Seen) != 2 {
		t.Fatalf("view = %+v, want the two seen addresses offered for confirmation", v)
	}
}

func TestWifiWatch_AnAddressYouDidNotConfirmThatUsesYourNameIsToldOnce(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	r.aps = []wifilook.AP{ap("home", routerBand, wifilook.WPA2), ap("home", meshBand, wifilook.WPA2), ap("home", twin, wifilook.WPA2), ap("Neighbour", "aa:aa:aa:00:00:01", wifilook.WPA2)}
	r.w.Tick(context.Background())
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %q, want one message", r.sent)
	}
	msg := r.sent[0]
	if !strings.Contains(msg, `"home"`) || !strings.Contains(msg, twin) || strings.Contains(msg, "Neighbour") || strings.Contains(msg, routerBand) {
		t.Fatalf("message = %q: it names the operator's own name and the unknown address, never another network or a confirmed address", msg)
	}
	// Another new address is told as it appears, one that was told is not told again until the gap has passed.
	r.aps = append(r.aps, ap("home", twin2, wifilook.WPA2))
	r.w.Tick(context.Background())
	if len(r.sent) != 2 || !strings.Contains(r.sent[1], twin2) || strings.Contains(r.sent[1], twin+",") {
		t.Fatalf("sent = %q, want the new address told on its own", r.sent)
	}
	r.now = r.now.Add(wifiAlertGap + time.Minute)
	r.w.Tick(context.Background())
	if len(r.sent) != 3 {
		t.Fatalf("sent = %d after the gap, want the standing look-alikes told again", len(r.sent))
	}
}

func TestWifiWatch_OnlyTheOperatorsOwnNameIsJudgedAndAHiddenNetworkNeverMatches(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	r.aps = []wifilook.AP{ap("", twin, wifilook.Open), ap("Home", twin2, wifilook.Open), ap("home ", twin2, wifilook.Open)}
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Fatalf("sent = %q: a hidden network and a name that differs in case or by a space are not the operator's name", r.sent)
	}
}

func TestWifiWatch_ADeliveryThatFailedIsTriedAgain(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	r.aps = append(r.aps, ap("home", twin, wifilook.WPA2))
	r.fail = true
	r.w.Tick(context.Background())
	r.fail = false
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %d, want the message tried again once it can be delivered", len(r.sent))
	}
}

func TestWifiWatch_AFailedLookIsAFailedPartNeverAnAllClear(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	r.err = errors.New("failed to list the Wi-Fi networks: nmcli failed: NetworkManager is not running")
	r.w.Tick(context.Background())
	r.w.Tick(context.Background())
	if len(r.errs) != 1 {
		t.Fatalf("errors told = %d, want the same one only once", len(r.errs))
	}
	if reading := r.w.Reading(); !reading.On || !reading.Failed {
		t.Fatalf("reading = %+v, want a failed part", reading)
	}
	r.err = nil
	r.w.Tick(context.Background())
	if reading := r.w.Reading(); reading.Failed || reading.LastGood.IsZero() {
		t.Fatalf("reading = %+v, want a working part once the look worked", reading)
	}
}

func TestWifiWatch_LooksAtNothingWhileSwitchedOffOrWithNoNameSet(t *testing.T) {
	r := newWifiRig(t)
	r.w.Tick(context.Background()) // on, but no name
	if r.looks != 0 {
		t.Fatalf("looked %d times with no network name set", r.looks)
	}
	r.confirmed(t)
	before := r.looks
	if err := r.cfg.Set(false); err != nil {
		t.Fatal(err)
	}
	r.aps = append(r.aps, ap("home", twin, wifilook.Open))
	r.w.Tick(context.Background())
	if r.looks != before || len(r.sent) != 0 || len(r.w.Snapshot(r.now).Lookalikes) != 0 {
		t.Fatal("a switched-off watch looked or told")
	}
}

func TestWifiWatch_ConfirmTakesOnlyAddressesTheLastLookSaw(t *testing.T) {
	r := newWifiRig(t)
	if err := r.cfg.AddNetwork("home"); err != nil {
		t.Fatal(err)
	}
	r.aps = []wifilook.AP{ap("home", routerBand, wifilook.WPA2), ap("Other", twin, wifilook.WPA2)}
	r.w.Tick(context.Background())
	for _, bad := range [][]string{{twin}, {"aa:bb:cc:00:00:ff"}, {"not-an-address"}, {}} {
		if err := r.w.Confirm("home", bad); err == nil {
			t.Errorf("Confirm(%v) was accepted: an address can be confirmed only if the last look saw it advertising that name", bad)
		}
	}
	if err := r.w.Confirm("nope", []string{routerBand}); err == nil {
		t.Error("an unknown network name was accepted")
	}
	if err := r.w.Confirm("home", []string{strings.ToUpper(routerBand)}); err != nil {
		t.Fatalf("a hardware address in capitals is the same address: %v", err)
	}
	if got := r.cfg.Networks()[0].Addresses; len(got) != 1 || got[0] != routerBand {
		t.Fatalf("confirmed = %v", got)
	}
}

func TestWifiWatch_ConfirmingAnAddressEndsItsLookalike(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	r.aps = append(r.aps, ap("home", twin, wifilook.WPA2))
	r.w.Tick(context.Background())
	if len(r.w.Snapshot(r.now).Lookalikes) != 1 {
		t.Fatal("setup: the twin should be a look-alike")
	}
	if err := r.w.Confirm("home", []string{twin}); err != nil {
		t.Fatal(err)
	}
	if got := r.w.Snapshot(r.now).Lookalikes; len(got) != 0 {
		t.Fatalf("look-alikes = %+v after the operator confirmed it", got)
	}
}

func TestWifiFindings_OneFindingForThisComputerWithTheWorstSecurityAndAVersionOfTheSet(t *testing.T) {
	if got := WifiFindings(WifiSnapshot{}); len(got) != 0 {
		t.Fatalf("findings = %+v for nothing", got)
	}
	two := WifiSnapshot{Lookalikes: []wifilook.Lookalike{{Name: "home", BSSID: twin, Signal: 40, Security: wifilook.WPA2}, {Name: "home", BSSID: twin2, Signal: 70, Security: wifilook.Open}}}
	got := WifiFindings(two)
	if len(got) != 1 || got[0].Device != HostDevice || got[0].Check != CheckWifiLookalike || got[0].Severity != SeverityHigh {
		t.Fatalf("findings = %+v, want one high finding (one copy is open)", got)
	}
	one := WifiFindings(WifiSnapshot{Lookalikes: two.Lookalikes[:1]})
	if one[0].Severity != SeverityMedium || one[0].Version == got[0].Version || one[0].Version == "" {
		t.Fatalf("one = %+v: severity medium, and the version changes with the set so an acceptance lapses when another copy appears", one[0])
	}
	if strings.Contains(got[0].Evidence, "Neighbour") || !strings.Contains(got[0].Evidence, twin) {
		t.Fatalf("evidence = %q", got[0].Evidence)
	}
	if CheckTitle(CheckWifiLookalike) == "" || RemediationFor(CheckWifiLookalike) == "" {
		t.Fatal("the check needs a title and a fix")
	}
	if _, ok := guides[CheckWifiLookalike]; !ok {
		t.Fatal("the check needs a fix guide")
	}
}

func TestWifiWatch_WhatItKeepsDoesNotGrowWithWhatIsAdvertised(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	var aps []wifilook.AP
	for i := 0; i < wifilook.MaxAPs; i++ {
		aps = append(aps, ap("home", "de:ad:be:ef:"+hex2(i/256)+":"+hex2(i%256), wifilook.Open))
	}
	r.aps = aps
	r.w.Tick(context.Background())
	if n := len(r.w.Snapshot(r.now).Lookalikes); n > maxWifiRecent {
		t.Fatalf("kept %d look-alikes, want at most %d", n, maxWifiRecent)
	}
	if len(r.sent) != 1 || strings.Count(r.sent[0], "de:ad:be:ef") > 3 {
		t.Fatalf("sent = %q: one message that lists a few addresses and counts the rest", r.sent)
	}
}

func hex2(n int) string { return string("0123456789abcdef"[n>>4]) + string("0123456789abcdef"[n&15]) }

func TestWifiConfig_ValidatesWhatTheOperatorTypesAndBoundsWhatItKeeps(t *testing.T) {
	cfg := NewWifiConfig(&memConfig{})
	for name, bad := range map[string]string{"empty": "", "blank": "   ", "over 32 bytes": strings.Repeat("x", 33), "control character": "a\x01b", "not UTF-8": "a\xffb"} {
		if err := cfg.AddNetwork(bad); err == nil {
			t.Errorf("%s was accepted as a network name", name)
		}
	}
	if err := cfg.AddNetwork("home"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.AddNetwork("home"); err == nil {
		t.Error("a name twice was accepted")
	}
	for i := 1; i < maxWifiNetworks; i++ {
		if err := cfg.AddNetwork("n" + hex2(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := cfg.AddNetwork("one too many"); err == nil {
		t.Error("more than the most networks was accepted")
	}
	if err := cfg.RemoveNetwork("home"); err != nil || len(cfg.Networks()) != maxWifiNetworks-1 {
		t.Fatalf("remove: %v, %d left", err, len(cfg.Networks()))
	}
	if err := cfg.RemoveNetwork("home"); err == nil {
		t.Error("removing a name that is not there was not an error")
	}
	if cfg.Enabled() {
		t.Error("the look is off until the operator turns it on")
	}
}

func TestWifiConfig_NetworksReturnsACopy(t *testing.T) {
	store := &memConfig{}
	cfg := NewWifiConfig(store)
	_ = cfg.AddNetwork("home")
	got := cfg.Networks()
	got[0].Name = "changed"
	if cfg.Networks()[0].Name != "home" {
		t.Fatal("a caller changed the stored networks through the returned slice")
	}
	_ = models.WifiNetwork{}
}

type fakeWifiSource struct{ snap WifiSnapshot }

func (f fakeWifiSource) Snapshot(time.Time) WifiSnapshot { return f.snap }

func TestRunReporter_AddsTheWifiLookalikeFindingOfThisComputer(t *testing.T) {
	r := NewRunReporter(nil).WithWifi(fakeWifiSource{WifiSnapshot{Lookalikes: []wifilook.Lookalike{{Name: "home", BSSID: twin, Security: wifilook.WPA2}}}})
	result := Result{}
	r.addWifi(&result)
	if len(result.Findings) != 1 || result.Findings[0].Check != CheckWifiLookalike || result.Findings[0].Device != HostDevice {
		t.Fatalf("findings = %+v", result.Findings)
	}
	NewRunReporter(nil).addWifi(&result) // a reporter without the watch adds nothing and does not fail
	if len(result.Findings) != 1 {
		t.Fatal("a reporter with no watch added a finding")
	}
}
