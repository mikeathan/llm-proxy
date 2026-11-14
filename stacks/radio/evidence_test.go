package radio

import (
	"context"
	"testing"
)

func classLogRows(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM class_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A neighbour sorted once and then not heard for a week has no rows in the 7-day window. It must keep its class (and its place in its list): the
// left join that makes every known device appear must not turn "not heard lately" into "not enough evidence".
func TestADeviceNotHeardForAWeekKeepsItsClassAndWritesNoLogRow(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	other := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	for i := 0; i < 14; i++ { // the neighbour is heard in 4 of 14 cycles and never strong: outside your walls
		if i < 4 {
			cycleOf(t, rec, r.now, dataAt(neighbour, -80), dataAt(other, -80))
			continue
		}
		cycleOf(t, rec, r.now, dataAt(other, -80))
	}
	if d, _ := classOf(t, store, neighbour); d.Class != "neighbour" {
		t.Fatalf("setup: %q", d.Class)
	}
	quiet := func(from, to int) { // another device keeps the sensor listening; this one is silent
		for day := from; day <= to; day++ {
			for i := 0; i < 13; i++ {
				cycleOf(t, rec, r.now.AddDate(0, 0, day), dataAt(other, -80))
			}
		}
	}
	quiet(1, 6) // its row (day 0) is still in the 7-day window, where the rules may rightly call a rare visitor a passer-by
	before, _ := classOf(t, store, neighbour)
	logged := classLogRows(t, store)
	quiet(7, 9) // now the row has left the window: no evidence at all
	after, _ := classOf(t, store, neighbour)
	if after.Class != before.Class || after.Class == "unknown" {
		t.Errorf("a silent week must not demote it: %q then %q", before.Class, after.Class)
	}
	if added := classLogRows(t, store) - logged; added != 0 {
		t.Errorf("%d new class_log rows: a device with no new evidence changes nothing", added)
	}
	r.mu.Lock()
	r.now = r.now.AddDate(0, 0, 9)
	r.mu.Unlock()
	var v AirDevices
	getQ(t, r, AirDevicesPath, "class="+before.Class, &v)
	found := false
	for _, d := range v.Devices {
		if d.CyclesHeard == 0 {
			found = true
			if d.Why != "not_heard_lately" {
				t.Errorf("a sorted device that went quiet is not waiting for evidence: %+v", d)
			}
		}
	}
	if !found {
		t.Errorf("the quiet device is still listed under its class: %+v", v.Devices)
	}
}

func TestAWindowCountsTheFramesOfTheOwnersDevices(t *testing.T) {
	s, _ := openTest(t, 0)
	rec := NewRecorder(fakeVault{}, s)
	ctx := context.Background()
	if err := rec.AddOwnerDevice(ctx, mineMAC, "phone", t0); err != nil {
		t.Fatal(err)
	}
	owner, err := rec.RecordSeen(ctx, mustParse(t, data(mineMAC)), t0)
	if err != nil || !owner {
		t.Fatalf("%v %v", owner, err)
	}
	owner, err = rec.RecordSeen(ctx, mustParse(t, data(neighbour)), t0)
	if err != nil || owner {
		t.Fatalf("%v %v", owner, err)
	}
}

// assocTo is an association request from a device to an access point.
func assocTo(from, ap MAC) []byte {
	return rtap(cat(hdr(0x00, 0, ap, from, ap), []byte{0, 0, 0, 0}))
}

func TestAnOwnerDeviceJoiningAnAccessPointThatIsNotYoursIsCountedWithNoAddress(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	ctx := context.Background()
	if err := rec.AddOwnerDevice(ctx, mineMAC, "phone", t0); err != nil {
		t.Fatal(err)
	}
	record(t, rec, assocTo(mineMAC, neighbourAP), t0)
	if _, ok := findingOf(digestOf(t, r), "radio_join_foreign_network"); ok {
		t.Fatal("with no confirmed access point of yours, every network is unknown: that is not a finding")
	}
	ownAP(t, store, 9, "home", "wpa2", "capable", true)
	if err := rec.RefreshOwnAPs(ctx); err != nil {
		t.Fatal(err)
	}
	record(t, rec, assocTo(mineMAC, neighbourAP), t0)
	f, ok := findingOf(digestOf(t, r), "radio_join_foreign_network")
	if !ok || f.Count != 1 || f.Tier != TierSeen {
		t.Errorf("%+v %v", f, ok)
	}
	// a neighbour's device joining a neighbour's access point is nobody's business here
	record(t, rec, assocTo(neighbour, neighbourAP), t0)
	if f, _ := findingOf(digestOf(t, r), "radio_join_foreign_network"); f.Count != 1 {
		t.Errorf("only the owner's own devices count: %+v", f)
	}
}

func TestAHiddenNetworkWithAStrongSignalIsNearTheHouse(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	record(t, rec, beacon("", rsnWPA2), t0) // a hidden SSID is a zero-length name; the test frames are heard at -60 dBm: strong
	f, ok := findingOf(digestOf(t, r), "radio_hidden_ap_near")
	if !ok || f.Count != 1 || f.Tier != TierEstimated {
		t.Errorf("%+v %v", f, ok)
	}
	d := digestOf(t, r)
	for _, c := range d.CouldNotSee {
		if c.ID == "radio_hidden_ap_near" || c.ID == "radio_join_foreign_network" {
			t.Errorf("both checks can run now: %+v", c)
		}
	}
}

func TestAHiddenNetworkHeardWeakIsNotNear(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	b := beacon("", rsnWPA2)
	dbm := -85
	b[14] = byte(int8(dbm))
	record(t, rec, b, t0)
	if _, ok := findingOf(digestOf(t, r), "radio_hidden_ap_near"); ok {
		t.Error("-85 dBm is far")
	}
}

func TestAWindowSummaryCountsTheOwnersFrames(t *testing.T) {
	s, _ := openTest(t, 0)
	rec := NewRecorder(fakeVault{}, s)
	if err := rec.AddOwnerDevice(context.Background(), mineMAC, "phone", t0); err != nil {
		t.Fatal(err)
	}
	raw := windowBytes(0, [][]byte{data(mineMAC), data(mineMAC), data(neighbour)}, true)
	sum, err := Ingest(context.Background(), rec, raw, WindowAsked{Channel: 60}, t0)
	if err != nil || sum.Recorded != 3 || sum.OwnFrames != 2 {
		t.Errorf("%+v %v", sum, err)
	}
}
