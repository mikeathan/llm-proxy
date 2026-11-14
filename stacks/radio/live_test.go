package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

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
	for _, d := range liveOf(t, r).Devices {
		if !d.Usual || d.Heard || d.Trend != TrendUnknown {
			t.Errorf("clearing forgets what was heard now; only the stored days remain, drawn as usual: %+v", d)
		}
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
	var now []LiveDevice
	for _, d := range liveOf(t, r).Devices {
		if !d.Usual {
			now = append(now, d)
		}
	}
	if len(now) != 1 {
		t.Fatalf("the silent one must leave the live picture: %+v", now)
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

func TestTheStoredDaysDrawWhatIsUsuallyHereButNotHeardNow(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	other := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	for i := 0; i < 3; i++ {
		cycleOf(t, rec, r.now, dataAt(neighbour, -60), dataAt(other, -75))
	}
	rec.ClearLive() // a restart: nothing in memory, the days on disk remain
	v := liveOf(t, r)
	if len(v.Devices) != 2 {
		t.Fatalf("both known devices are drawn from their days: %+v", v.Devices)
	}
	for _, d := range v.Devices {
		if !d.Usual || d.Heard || d.Trend != TrendUnknown || d.NearM <= 0 || d.FarM <= d.NearM || d.Label != "" {
			t.Errorf("a usual dot has a range, no trend and no label: %+v", d)
		}
	}
	if v.Devices[0].Dbm != -60 || v.Devices[1].Dbm != -75 {
		t.Errorf("a usual dot sits at its mean signal: %+v", v.Devices)
	}
	cycleOf(t, rec, r.now, dataAt(neighbour, -60))
	var usual, now int
	for _, d := range liveOf(t, r).Devices {
		if d.Usual {
			usual++
		} else {
			now++
		}
	}
	if usual != 1 || now != 1 {
		t.Errorf("a device heard now is drawn once, as live: usual %d now %d", usual, now)
	}
}

func TestADeviceMuchCloserThanItsOwnUsualIsFlagged(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	steady := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	for i := 0; i < 12; i++ {
		cycleOf(t, rec, r.now, dataAt(neighbour, -80), dataAt(steady, -60))
	}
	rec.BeginCycle()
	record(t, rec, dataAt(neighbour, -50), r.now)
	record(t, rec, dataAt(steady, -61), r.now)
	byDbm := map[int]LiveDevice{}
	for _, d := range liveOf(t, r).Devices {
		byDbm[d.Dbm] = d
	}
	moved := byDbm[-50]
	if !moved.Unusual || moved.UsualDbm != -80 || moved.ShiftDB != 30 {
		t.Errorf("30 dB above its usual is unusual: %+v", moved)
	}
	if calm := byDbm[-61]; calm.Unusual || calm.UsualDbm != -60 {
		t.Errorf("1 dB is not: %+v", calm)
	}
}

func TestADeviceWithTooFewStoredSamplesIsNeverCalledUnusual(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, dataAt(neighbour, -80))
	rec.BeginCycle()
	record(t, rec, dataAt(neighbour, -40), r.now)
	if d := liveOf(t, r).Devices[0]; d.Unusual || d.UsualDbm != 0 {
		t.Errorf("one old sample is no baseline: %+v", d)
	}
}

func TestAnOwnerDeviceHoursCarryADistanceRangeWhenHeard(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Phone", r.now); err != nil {
		t.Fatal(err)
	}
	cycleOf(t, rec, r.now, dataAt(neighbour, -55))
	var v PresenceView
	getQ(t, r, PresencePath, "", &v)
	if len(v.Devices) != 1 {
		t.Fatalf("%+v", v)
	}
	var here, other int
	for _, h := range v.Devices[0].Hours {
		if h.State == hourHere {
			here++
			if h.Dbm != -55 || h.NearM <= 0 || h.FarM <= h.NearM {
				t.Errorf("a heard hour carries its mean signal and range: %+v", h)
			}
		} else {
			other++
			if h.Dbm != 0 || h.NearM != 0 {
				t.Errorf("an hour with no reading carries no distance: %+v", h)
			}
		}
	}
	if here != 1 || other == 0 {
		t.Errorf("here %d other %d", here, other)
	}
}

func trackOf(t *testing.T, r *listenRig, id string) (TrackView, int) {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: TrackPath, Params: map[string]string{"id": id}})
	if err != nil {
		t.Fatal(err)
	}
	var v TrackView
	if resp.Status == http.StatusOK {
		if err := json.Unmarshal(resp.Body, &v); err != nil {
			t.Fatal(err)
		}
	}
	return v, resp.Status
}

func cyclesAt(t *testing.T, rec *Recorder, r *listenRig, mins []int, signals []float64) {
	t.Helper()
	for i, m := range mins {
		at := r.now.Add(time.Duration(m) * time.Minute)
		if signals[i] == 0 {
			cycleOf(t, rec, at)
			continue
		}
		cycleOf(t, rec, at, dataAt(neighbour, int(signals[i])))
	}
}

func TestATrackKeepsEveryCycleHeardInMemoryAndMarksTheGaps(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5, 10, 15, 20, 25}, []float64{-80, -70, 0, 0, 0, -60})
	id := liveOf(t, r).Devices[0].ID
	v, code := trackOf(t, r, id)
	if code != http.StatusOK || len(v.Points) != 3 || v.Stored {
		t.Fatalf("three points from memory expected: %d %+v", code, v)
	}
	if v.Points[0].Dbm != -80 || v.Points[2].Dbm != -60 || v.Points[0].At >= v.Points[2].At {
		t.Errorf("oldest first: %+v", v.Points)
	}
	if v.Points[1].Run != v.Points[0].Run || v.Points[2].Run == v.Points[1].Run {
		t.Errorf("three silent cycles are a gap, a new run: %+v", v.Points)
	}
	if v.Points[2].NearM >= v.Points[0].NearM || v.Trend != TrendCloser {
		t.Errorf("a stronger signal is nearer: %+v", v)
	}
	if strings.Contains(string(mustJSON(t, v)), "b8:27") {
		t.Error("an address leaked into a track")
	}
	if n, _ := store.count("air_track"); n != 0 {
		t.Error("with the setting off nothing is written")
	}
	rec.ClearLive()
	if _, code := trackOf(t, r, id); code != http.StatusNotFound {
		t.Errorf("a run that ends leaves no track while the setting is off: %d", code)
	}
}

func TestATrackIsBoundedInLengthAndAge(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	for i := 0; i < liveTrackCycles+20; i++ {
		cycleOf(t, rec, r.now.Add(time.Duration(i)*time.Minute), dataAt(neighbour, -60))
	}
	id := liveOf(t, r).Devices[0].ID
	if v, _ := trackOf(t, r, id); len(v.Points) != liveTrackCycles {
		t.Errorf("a track keeps at most %d cycles, got %d", liveTrackCycles, len(v.Points))
	}
	for i := 0; i < liveTrackCycles+1; i++ {
		cycleOf(t, rec, r.now.Add(time.Duration(liveTrackCycles+20+i)*time.Minute))
	}
	if _, code := trackOf(t, r, id); code != http.StatusNotFound {
		t.Errorf("a device silent for a whole track length is forgotten: %d", code)
	}
}

func TestLiveCarriesTheEvidenceTheTailAndTheCycleTiming(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Sam's phone", r.now); err != nil {
		t.Fatal(err)
	}
	other := MAC{0xb8, 0x27, 0xeb, 7, 7, 7}
	for i := 0; i < 3; i++ {
		cycleOf(t, rec, r.now.Add(time.Duration(i)*time.Minute), dataAt(neighbour, -45), dataAt(other, -70-i))
	}
	v := liveOf(t, r)
	if v.LastCycleAt != r.now.Add(2*time.Minute).Unix() || v.CycleSeconds <= 0 || v.MinListened <= 0 || v.Listened < 3 {
		t.Errorf("cycle timing: %+v", v)
	}
	var mine, theirs int
	for _, d := range v.Devices {
		switch {
		case d.Mine:
			mine++
			if d.Detail != nil {
				t.Error("the owner's device carries no sorting evidence")
			}
		default:
			theirs++
			if d.Detail == nil || d.Detail.Why == "" || d.Detail.ID != d.ID || len(d.Detail.Channels) == 0 {
				t.Errorf("a neighbour carries the evidence behind its kind: %+v", d.Detail)
			}
			if len(d.TailM) != 3 || d.TailM[0] >= d.TailM[2] {
				t.Errorf("tail oldest first, a weaker signal is farther: %v", d.TailM)
			}
		}
	}
	if mine != 1 || theirs != 1 {
		t.Errorf("mine %d theirs %d", mine, theirs)
	}
}
