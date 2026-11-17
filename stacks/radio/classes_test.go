package radio

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func cycleOf(t *testing.T, r *Recorder, at time.Time, frames ...[]byte) {
	t.Helper()
	r.BeginCycle()
	for _, f := range frames {
		record(t, r, f, at)
	}
	if err := r.EndCycle(context.Background(), at); err != nil {
		t.Fatal(err)
	}
}

func classOf(t *testing.T, s *Store, m MAC) (Device, bool) {
	t.Helper()
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, m[:])
	d, ok, err := s.Device(context.Background(), tag)
	if err != nil {
		t.Fatal(err)
	}
	return d, ok
}

func TestRulesClassifyByEvidenceAndAreUnknownWhenThin(t *testing.T) {
	r := DefaultRules()
	cases := []struct {
		name string
		f    Features
		want Class
		why  Why
	}{
		{"too little listening", Features{CyclesListened: 3, CyclesHeard: 3, Share: 1}, ClassUnknown, WhyNeedsMore},
		{"heard too rarely", Features{CyclesListened: 50, CyclesHeard: 1, Share: .02}, ClassUnknown, WhyNeedsMore},
		{"always here and steady", Features{CyclesListened: 50, CyclesHeard: 49, Share: .98, HasSignal: true, SignalStd: 2, SignalMax: -50, Days: 3}, ClassInfrastructure, WhyAlwaysSteady},
		{"a random address is not told as steady, but always here is always on (version 2)", Features{CyclesListened: 50, CyclesHeard: 49, Share: .98, HasSignal: true, SignalStd: 2, SignalMax: -50, Days: 3, Random: true}, ClassInfrastructure, WhyAlwaysVaries},
		{"joins the owner's network, always here", Features{CyclesListened: 50, CyclesHeard: 49, Share: .98, JoinsOwn: true}, ClassInfrastructure, WhyJoinsAlways},
		{"joins the owner's network, comes and goes", Features{CyclesListened: 50, CyclesHeard: 20, Share: .4, JoinsOwn: true}, ClassPersonal, WhyJoinsSometimes},
		{"a passer-by", Features{CyclesListened: 100, CyclesHeard: 4, Share: .04, Days: 1, HasSignal: true, SignalMax: -70}, ClassTransient, WhyBrief},
		{"weak all along", Features{CyclesListened: 50, CyclesHeard: 30, Share: .6, Days: 3, HasSignal: true, SignalMax: -82, SignalStd: 6}, ClassNeighbour, WhyNeverStrong},
		{"strong and intermittent", Features{CyclesListened: 50, CyclesHeard: 25, Share: .5, Days: 3, HasSignal: true, SignalMax: -45, SignalStd: 9}, ClassPersonal, WhyStrongOnAndOff},
		{"in between: no rule fits", Features{CyclesListened: 50, CyclesHeard: 25, Share: .5, Days: 3, HasSignal: true, SignalMax: -68, SignalStd: 9}, ClassUnknown, WhyNoRuleFits},
	}
	for _, c := range cases {
		if got, why := r.Classify(c.f); got != c.want || why != c.why {
			t.Errorf("%s: %s/%s, want %s/%s", c.name, got, why, c.want, c.why)
		}
	}
	if !r.Provisional {
		t.Error("the rules are not fitted to measured data, and must say so")
	}
}

func TestACycleCountsADeviceOnceAndTheClassifierFollowsTheEvidence(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	// A steady device heard in every cycle, and a visitor heard in one.
	for i := 0; i < 14; i++ {
		at := t0.Add(time.Duration(i) * 5 * time.Minute)
		frames := [][]byte{data(neighbour), data(neighbour), data(neighbour)}
		if i == 2 {
			frames = append(frames, data(MAC{0x10, 1, 2, 3, 4, 5}))
		}
		cycleOf(t, r, at, frames...)
	}
	d, ok := classOf(t, s, neighbour)
	if !ok || d.Class != string(ClassInfrastructure) {
		t.Fatalf("a device heard in every cycle at a steady signal: %+v", d)
	}
	visitor, ok := classOf(t, s, MAC{0x10, 1, 2, 3, 4, 5})
	if !ok || visitor.Class != string(ClassUnknown) {
		t.Errorf("one cycle of evidence is Unknown, not a guess: %+v", visitor)
	}
	changes, err := s.RecentClassChanges(ctx, dayNumber(t0)-1, 10)
	if err != nil || len(changes) == 0 {
		t.Fatalf("a class change is logged with its reason: %+v %v", changes, err)
	}
	for _, c := range changes {
		if c.Reason == "" || c.Version != 2 {
			t.Errorf("%+v", c)
		}
	}
	rows, listened, _ := s.FeatureRows(ctx, dayNumber(t0))
	if listened[dayNumber(t0)] != 14 {
		t.Errorf("the listened cycles of the day: %v", listened)
	}
	for _, row := range rows {
		if row.Days != 1 {
			t.Errorf("one row a day, however many cycles: %+v", row)
		}
	}
}

func TestADroppedCycleIsNotCounted(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	r.BeginCycle()
	record(t, r, data(neighbour), t0)
	r.DropCycle()
	if err := r.EndCycle(context.Background(), t0); err != nil {
		t.Fatal(err)
	}
	if air, _ := s.airDays(context.Background(), 0); len(air) != 0 {
		t.Errorf("a half cycle would bias every share: %v", air)
	}
}

func TestTheOwnersWordOnAClassIsNeverOverwritten(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	tag, _ := fakeVault{}.Tag(ctx, tagDevice, mineMAC[:])
	sealed, _ := fakeVault{}.Seal(ctx, sealOwner, mineMAC[:])
	if err := s.SeenDevice(ctx, Seen{Tag: tag, Owner: true, Sealed: sealed, Label: "tag", Class: string(ClassPersonal), At: t0}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		cycleOf(t, r, t0.Add(time.Duration(i)*5*time.Minute), data(mineMAC), data(mineMAC))
	}
	if d, _ := classOf(t, s, mineMAC); d.Class != string(ClassPersonal) {
		t.Errorf("the owner said personal; the rules must not change it: %+v", d)
	}
}

func TestAnAccessPointWithTheOwnersNameIsOnlyProposedAndJoiningItNeedsConfirmation(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	r.SetOwnNetworks([]string{"home"})
	_ = r.RefreshOwnAPs(ctx)
	record(t, r, beacon("home"), t0) // from macAP; anyone could broadcast "home"
	assoc := rtap(cat(hdr(0x00, 0, macAP, macPhone, macAP), []byte{0x11, 0x04, 10, 0}, ie(0, []byte("home")...)))
	cycleOf(t, r, t0, assoc)
	phone, _ := classOf(t, s, macPhone)
	if phone.Owner {
		t.Fatal("not the owner's")
	}
	aps, err := s.OwnAPs(ctx)
	if err != nil || len(aps) != 1 || aps[0].Confirmed || aps[0].Name != "home" {
		t.Fatalf("a name match proposes: %+v %v", aps, err)
	}
	if joined(t, s, macPhone) {
		t.Error("joining an unconfirmed access point must not count")
	}
	if ok, err := s.SetOwnAPConfirmed(ctx, aps[0].Tag, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	_ = r.RefreshOwnAPs(ctx)
	cycleOf(t, r, t0, assoc)
	if !joined(t, s, macPhone) {
		t.Error("joining a confirmed access point is the strongest evidence")
	}
	other := rtap(cat(hdr(0x00, 0, neighbourAP, macPhone, neighbourAP), []byte{0x11, 0x04, 10, 0}, ie(0, []byte("sky")...)))
	if err := r.Record(ctx, mustParse(t, other), t0); err != nil {
		t.Fatal(err)
	}
}

func mustParse(t *testing.T, b []byte) Frame {
	t.Helper()
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func joined(t *testing.T, s *Store, m MAC) bool {
	t.Helper()
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, m[:])
	var j bool
	if err := s.db.QueryRow(`SELECT joined_own FROM devices WHERE tag = ?`, tag).Scan(&j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestForgetNeighboursKeepsTheOwnersAccessPointsMatching(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	r.SetOwnNetworks([]string{"home"})
	record(t, r, beacon("home"), t0)
	aps, _ := s.OwnAPs(ctx)
	_, _ = s.SetOwnAPConfirmed(ctx, aps[0].Tag, true)
	cycleOf(t, r, t0, data(neighbour))
	if err := r.ForgetNeighbours(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := s.OwnAPs(ctx)
	want, _ := fakeVault{}.Tag(ctx, purpose(tagAP, 1), macAP[:])
	if len(after) != 1 || string(after[0].Tag) != string(want) || !after[0].Confirmed {
		t.Errorf("the confirmed access point must follow the new epoch: %+v", after)
	}
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM daily`).Scan(&n)
	if n != 0 {
		t.Errorf("a neighbour's day rows go with the rotation: %d", n)
	}
}

// Version 2 is shaped by what vertex's air showed on 2026-11-16 (45 devices over 95 cycles, docs/PLANS/cross-cutting/radio-report-wording-and-device-patterns.md):
// the owner's own access points sorted as unknown, passing routers sorted as personal, and seven always-here devices with no rule at all.
func TestVersionTwoSortsAccessPointsAndAlwaysHereDevicesTheWayVertexsAirNeeds(t *testing.T) {
	r := DefaultRules()
	if r.Version != 2 || !r.Provisional || len(r.SmartHomeMakers) == 0 {
		t.Fatalf("version 2 is provisional and carries its maker list: %+v", r)
	}
	always := Features{CyclesListened: 95, CyclesHeard: 95, Share: 1, Days: 3, HasSignal: true, SignalMean: -64, SignalMax: -50, SignalStd: 8}
	cases := []struct {
		name string
		f    Features
		want Class
		why  Why
	}{
		{"the owner's access point, always here, signal not steady", with(always, func(f *Features) { f.AccessPoint = true; f.Maker = "ASUSTek COMPUTER" }), ClassInfrastructure, WhyAccessPointAlways},
		{"an access point on a random address (a virtual network), always here", with(always, func(f *Features) { f.AccessPoint = true; f.Random = true }), ClassInfrastructure, WhyAccessPointAlways},
		{"an access point that is always here but never strong is a neighbour's", with(always, func(f *Features) { f.AccessPoint = true; f.SignalMax = -82; f.SignalMean = -85 }), ClassNeighbour, WhyNeverStrong},
		{"a router that comes and goes is not a personal device", Features{CyclesListened: 95, CyclesHeard: 31, Share: .33, Days: 3, HasSignal: true, SignalMean: -62, SignalMax: -45, SignalStd: 9, AccessPoint: true, Maker: "Arcadyan"}, ClassNeighbour, WhyAccessPointSometimes},
		{"always here with a signal that is not steady", with(always, func(f *Features) {}), ClassInfrastructure, WhyAlwaysVaries},
		{"always here but never strong is outside the walls, not the house's infrastructure", with(always, func(f *Features) { f.SignalMax = -80; f.SignalMean = -84 }), ClassNeighbour, WhyNeverStrong},
		{"always here on a random address", with(always, func(f *Features) { f.Random = true }), ClassInfrastructure, WhyAlwaysVaries},
		{"a smart-home module, heard part of the time", Features{CyclesListened: 95, CyclesHeard: 32, Share: .34, Days: 3, HasSignal: true, SignalMean: -66, SignalMax: -50, SignalStd: 7, Maker: "Espressif Inc."}, ClassInfrastructure, WhySmartHomeMaker},
		{"a smart-home module far away stays a neighbour's", Features{CyclesListened: 95, CyclesHeard: 32, Share: .34, Days: 3, HasSignal: true, SignalMean: -85, SignalMax: -80, SignalStd: 3, Maker: "Espressif Inc."}, ClassNeighbour, WhyNeverStrong},
		{"a phone with a random address is still personal", Features{CyclesListened: 95, CyclesHeard: 40, Share: .42, Days: 3, HasSignal: true, SignalMean: -52, SignalMax: -40, SignalStd: 8, Random: true}, ClassPersonal, WhyStrongOnAndOff},
		{"a smart-home maker on a random address is not told apart by maker", Features{CyclesListened: 95, CyclesHeard: 32, Share: .34, Days: 3, HasSignal: true, SignalMean: -66, SignalMax: -50, SignalStd: 7, Maker: "Espressif Inc.", Random: true}, ClassPersonal, WhyStrongOnAndOff},
	}
	for _, c := range cases {
		if got, why := r.Classify(c.f); got != c.want || why != c.why {
			t.Errorf("%s: %s/%s, want %s/%s", c.name, got, why, c.want, c.why)
		}
	}
}

func with(f Features, change func(*Features)) Features {
	change(&f)
	return f
}

// Rules are data and a version change is published, not applied silently: a database that holds version 1 moves to the version this code carries,
// every device is looked at again, and each class that CHANGED is logged with the new version and its reason. A device whose class stays the same is
// moved to the new version with no log line ("personal to personal" would fill the page with noise).
func TestAStoreOnVersionOneMovesToVersionTwoAndOnlyARealChangeIsLogged(t *testing.T) {
	s, _ := openTest(t, 0)
	rec := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	wobbly, steady := MAC{0x02, 0x11, 0x22, 0x33, 0x44, 0x01}, MAC{0x02, 0x11, 0x22, 0x33, 0x44, 0x02}
	for i := 0; i < 14; i++ { // both are heard in every cycle; one's signal swings, the other's does not
		cycleOf(t, rec, t0, dataAt(wobbly, -40+20*(i%2)), dataAt(steady, -50))
	}
	// Put the store back as version 1 left it: the wobbly device unknown (no rule fitted it), the steady one infrastructure, no log.
	v1 := DefaultRules()
	v1.Version, v1.SmartHomeMakers = 1, nil
	if err := s.PutDoc(ctx, "classifier_v1", v1, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMetaInt(ctx, "classifier_current", 1); err != nil {
		t.Fatal(err)
	}
	wt, _ := fakeVault{}.Tag(ctx, tagDevice, wobbly[:])
	st, _ := fakeVault{}.Tag(ctx, tagDevice, steady[:])
	if err := s.SetClass(ctx, wt, false, "infrastructure", "unknown", 1, string(WhyNoRuleFits), t0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClass(ctx, st, false, "unknown", "infrastructure", 1, string(WhyAlwaysSteady), t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM class_log`); err != nil {
		t.Fatal(err)
	}

	got, err := s.CurrentRules(ctx, t0)
	if err != nil || got.Version != 2 {
		t.Fatalf("the rules in force must be this code's version 2: %+v, %v", got, err)
	}
	if v, _ := s.MetaInt(ctx, "classifier_current"); v != 2 {
		t.Errorf("classifier_current = %d, want 2", v)
	}
	if changed, err := s.ClassifyAll(ctx, t0); err != nil || changed != 1 {
		t.Fatalf("exactly one device changes class: %d, %v", changed, err)
	}
	log, err := s.RecentClassChanges(ctx, dayNumber(t0)-1, 10)
	if err != nil || len(log) != 1 || log[0].From != "unknown" || log[0].To != "infrastructure" || log[0].Version != 2 || log[0].Reason != string(WhyAlwaysVaries) {
		t.Errorf("one logged change, with version 2 and its reason: %+v, %v", log, err)
	}
	for name, tag := range map[string][]byte{"wobbly": wt, "steady": st} {
		if d, _, _ := s.Device(ctx, tag); d.Class != "infrastructure" {
			t.Errorf("%s ended as %q", name, d.Class)
		}
	}
	var stale int
	_ = s.db.QueryRow(`SELECT count(*) FROM devices WHERE owner = 0 AND class_ver != 2`).Scan(&stale)
	if stale != 0 {
		t.Errorf("%d devices are still on an old rules version", stale)
	}
}

// A Why with no words on the screen shows as a raw id. Every reason the rules can give has its plain words in the page's table.
func TestEveryReasonTheRulesCanGiveHasItsWordsOnThePage(t *testing.T) {
	page, err := os.ReadFile("../../frontend/src/constants/radio.ts")
	if err != nil {
		t.Skip("the frontend is not beside the stack")
	}
	for _, w := range Whys() {
		if !strings.Contains(string(page), "  "+string(w)+": ") {
			t.Errorf("reason %q has no words in frontend/src/constants/radio.ts (RADIO_REASONS)", w)
		}
	}
}

// Every maker on the smart-home list must name something in the embedded registry, or the rule matches nothing and nobody would notice.
func TestEverySmartHomeMakerOnTheListMatchesTheRegistry(t *testing.T) {
	f, err := os.Open("oui_data.tsv.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	table := strings.ToLower(string(raw))
	for _, part := range DefaultRules().SmartHomeMakers {
		if !strings.Contains(table, part) {
			t.Errorf("%q matches no maker in the registry: drop it from the list", part)
		}
	}
}
