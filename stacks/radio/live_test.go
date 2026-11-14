package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func TestDistanceRangeWidensWithWeakerSignalAndKeepsTheOrderOfTheModel(t *testing.T) {
	m := DefaultDistance()
	n1, f1 := m.Range(-50, 6)
	n2, f2 := m.Range(-70, 6)
	if !(n1 < f1 && n2 < f2) {
		t.Fatalf("near must be below far: %v-%v %v-%v", n1, f1, n2, f2)
	}
	if !(n2 > n1 && f2 > f1) {
		t.Errorf("a weaker signal is farther: %v-%v then %v-%v", n1, f1, n2, f2)
	}
	if n, _ := m.Range(-10, 6); n < 1 {
		t.Errorf("never closer than a metre, got %v", n)
	}
	n5, _ := m.Range(-60, 36) // 5 GHz loses more at one metre, so the same signal means closer
	n24, _ := m.Range(-60, 6)
	if n5 >= n24 {
		t.Errorf("5 GHz at -60 should be nearer than 2.4 GHz at -60: %v vs %v", n5, n24)
	}
}

func TestRingsFollowTheMiddleOfTheRange(t *testing.T) {
	for _, c := range []struct {
		near, far float64
		want      string
	}{{1, 2, RingRoom}, {3, 9, RingHouse}, {10, 30, RingNearby}, {40, 120, RingFar}} {
		if got := RingOf(c.near, c.far); got != c.want {
			t.Errorf("RingOf(%v,%v) = %q, want %q", c.near, c.far, got, c.want)
		}
	}
}

func TestTrendNeedsThreeSamplesAndFourDecibels(t *testing.T) {
	cases := []struct {
		trail []float64
		want  string
	}{
		{[]float64{-70, -60}, TrendUnknown},
		{[]float64{-70, -70, -62}, TrendCloser},
		{[]float64{-50, -50, -58}, TrendAway},
		{[]float64{-60, -61, -60, -59}, TrendSteady},
	}
	for _, c := range cases {
		if got := TrendOf(c.trail); got != c.want {
			t.Errorf("TrendOf(%v) = %q, want %q", c.trail, got, c.want)
		}
	}
}

func liveOf(t *testing.T, r *listenRig) LiveView {
	t.Helper()
	var v LiveView
	getQ(t, r, LivePath, "", &v)
	return v
}

func TestLiveShowsTheOpenCycleAndKeepsOnlyMemory(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, dataAt(neighbour, -80))
	cycleOf(t, rec, r.now, dataAt(neighbour, -80))
	rec.BeginCycle()
	record(t, rec, dataAt(neighbour, -50), r.now) // the window in progress already moves the dot
	v := liveOf(t, r)
	if len(v.Devices) != 1 {
		t.Fatalf("one neighbour expected: %+v", v)
	}
	d := v.Devices[0]
	if d.Dbm != -50 || !d.Heard || d.Trend != TrendCloser || d.Mine {
		t.Errorf("live neighbour: %+v", d)
	}
	if d.NearM <= 0 || d.FarM <= d.NearM || d.Ring == "" || d.Angle < 0 || d.Angle >= 360 {
		t.Errorf("the distance is a range with a ring and a stable angle: %+v", d)
	}
	if strings.Contains(string(mustJSON(t, v)), "b8:27") {
		t.Error("an address leaked into the live answer")
	}
	var n int
	_ = store.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name LIKE '%live%'`).Scan(&n)
	if n != 0 {
		t.Error("the live picture must never have a table")
	}
	again := liveOf(t, r).Devices[0]
	if again.Angle != d.Angle {
		t.Errorf("the angle must be stable between looks: %v then %v", d.Angle, again.Angle)
	}
	rec.DropCycle()
	rec.ClearLive()
	if got := liveOf(t, r); len(got.Devices) != 0 {
		t.Errorf("clearing forgets everything: %+v", got)
	}
}

func TestLiveLabelsOnlyTheOwnersDevices(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Sam's phone", r.now); err != nil {
		t.Fatal(err)
	}
	other := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	cycleOf(t, rec, r.now, dataAt(neighbour, -45), dataAt(other, -70))
	v := liveOf(t, r)
	var mine, theirs int
	for _, d := range v.Devices {
		if d.Mine {
			mine++
			if d.Label != "Sam's phone" {
				t.Errorf("label %q", d.Label)
			}
		} else {
			theirs++
			if d.Label != "" {
				t.Errorf("a neighbour never gets a label: %q", d.Label)
			}
		}
	}
	if mine != 1 || theirs != 1 {
		t.Errorf("mine %d theirs %d", mine, theirs)
	}
}

func TestADeviceNotHeardForSomeCyclesLeavesTheLivePicture(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	other := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	cycleOf(t, rec, r.now, dataAt(neighbour, -60), dataAt(other, -60))
	for i := 0; i < liveKeepCycles+1; i++ {
		cycleOf(t, rec, r.now, dataAt(other, -60))
	}
	v := liveOf(t, r)
	if len(v.Devices) != 1 {
		t.Fatalf("the silent one must be gone: %+v", v.Devices)
	}
}

func TestCalibrationMovesEveryDistanceByTheOffsetAndIsKept(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Phone", r.now); err != nil {
		t.Fatal(err)
	}
	cycleOf(t, rec, r.now, dataAt(neighbour, -52))
	before := liveOf(t, r).Devices[0]
	id := before.ID
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: CalibratePath, Params: map[string]string{"id": id}})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("calibrate: %+v %v", resp, err)
	}
	after := liveOf(t, r)
	if !after.Model.Calibrated {
		t.Error("the model must say it is calibrated")
	}
	if after.Devices[0].FarM >= before.FarM {
		t.Errorf("a phone put 1 m away reading -52 dBm means the model was too generous: %+v then %+v", before, after.Devices[0])
	}
	resp, _ = r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: CalibratePath, Params: map[string]string{"id": "00000000"}})
	if resp.Status != http.StatusNotFound {
		t.Errorf("only a device of the owner's that is heard can calibrate: %d", resp.Status)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
