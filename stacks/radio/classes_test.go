package radio

import (
	"context"
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
		{"a random address is never infrastructure on steadiness alone", Features{CyclesListened: 50, CyclesHeard: 49, Share: .98, HasSignal: true, SignalStd: 2, SignalMax: -50, Days: 3, Random: true}, ClassUnknown, WhyNoRuleFits},
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
		t.Error("version 1 is not measured, and must say so")
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
		if c.Reason == "" || c.Version != 1 {
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
