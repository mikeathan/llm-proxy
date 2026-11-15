package radio

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func keep(t *testing.T, r *listenRig, hours string) int {
	t.Helper()
	resp, err := call(t, r, http.MethodPut, KeepPath, `{"track_hours":`+hours+`}`)
	if err != nil {
		t.Fatal(err)
	}
	return resp.Status
}

func TestNothingFinerThanADayIsWrittenByDefault(t *testing.T) {
	r := devRig(t)
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5, 10}, []float64{-70, -65, -60})
	if n, _ := store.count("air_track"); n != 0 {
		t.Fatalf("the default stores no track: %d rows", n)
	}
	var v KeepView
	getQ(t, r, KeepPath, "", &v)
	if v.TrackHours != 0 || len(v.Choices) != 3 {
		t.Errorf("off by default, three choices: %+v", v)
	}
}

func TestTrackOnKeepsPointsAndAnswersFromDiskAfterTheRunIsForgotten(t *testing.T) {
	r := devRig(t)
	if keep(t, r, "24") != http.StatusOK {
		t.Fatal("24 hours is accepted")
	}
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5, 10}, []float64{-80, -70, -60})
	if n, _ := store.count("air_track"); n != 3 {
		t.Fatalf("three points on disk: %d", n)
	}
	id := liveOf(t, r).Devices[0].ID
	rec.ClearLive() // a restart leaves no memory
	v, code := trackOf(t, r, id)
	if code != http.StatusOK || !v.Stored || len(v.Points) != 3 || v.Points[0].Dbm != -80 {
		t.Fatalf("the track comes back from disk: %d %+v", code, v)
	}
	var k KeepView
	getQ(t, r, KeepPath, "", &k)
	if k.TrackHours != 24 || k.StoredPoints != 3 {
		t.Errorf("the panel says what is kept: %+v", k)
	}
}

func TestTurningTheTrackOffDeletesItAtOnce(t *testing.T) {
	r := devRig(t)
	keep(t, r, "48")
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5}, []float64{-70, -60})
	if keep(t, r, "0") != http.StatusOK {
		t.Fatal("off is accepted")
	}
	if n, _ := store.count("air_track"); n != 0 {
		t.Errorf("off deletes in the same request: %d rows", n)
	}
	cyclesAt(t, rec, r, []int{10}, []float64{-60})
	if n, _ := store.count("air_track"); n != 0 {
		t.Errorf("off stops writing: %d rows", n)
	}
}

func TestDeletingStoredTracksKeepsTheSettingOn(t *testing.T) {
	r := devRig(t)
	keep(t, r, "24")
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5}, []float64{-70, -60})
	resp, err := call(t, r, http.MethodDelete, KeepPath, "")
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("%+v %v", resp, err)
	}
	if n, _ := store.count("air_track"); n != 0 {
		t.Errorf("deleted: %d", n)
	}
	var k KeepView
	getQ(t, r, KeepPath, "", &k)
	if k.TrackHours != 24 {
		t.Errorf("deleting does not change the setting: %+v", k)
	}
}

func TestForgetNeighboursDeletesTheirTracksAndKeepsTheOwnersOnlyByRetagging(t *testing.T) {
	r := devRig(t)
	keep(t, r, "24")
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5}, []float64{-70, -60})
	if err := rec.ForgetNeighbours(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.count("air_track"); n != 0 {
		t.Errorf("forgetting neighbours removes every track: %d", n)
	}
}

func TestKeepingRefusesAnythingButTheThreeChoices(t *testing.T) {
	r := devRig(t)
	for _, h := range []string{"12", "72", "-1", "336"} {
		if got := keep(t, r, h); got != http.StatusBadRequest {
			t.Errorf("%s hours: %d", h, got)
		}
	}
	resp, _ := call(t, r, http.MethodPut, KeepPath, `{"track_hours":24,"extra":1}`)
	if resp.Status != http.StatusBadRequest {
		t.Errorf("an unknown field is refused: %d", resp.Status)
	}
}

func TestPrunedAndCappedTracks(t *testing.T) {
	r := devRig(t)
	_, store, _ := r.s.recorder(r.s.h)
	ctx := t.Context()
	now := r.now
	tag := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if err := store.AddTrack(ctx, now.Add(-30*time.Hour), []trackRow{{tag: tag, sig: -60, channel: 6}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTrack(ctx, now.Add(-1*time.Hour), []trackRow{{tag: tag, sig: -60, channel: 6}}); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneTracks(ctx, now, 24); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.count("air_track"); n != 1 {
		t.Errorf("a point past its age goes: %d", n)
	}
	rows := make([]trackRow, 0, 1000)
	for i := 0; i < 1000; i++ {
		rows = append(rows, trackRow{tag: []byte{byte(i), byte(i >> 8), 0, 0, 0, 0, 0, 1}, sig: -70, channel: 1})
	}
	store.trackRowCap = 500
	if err := store.AddTrack(ctx, now, rows); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneTracks(ctx, now, 24); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.count("air_track"); n > 500 {
		t.Errorf("the row cap holds: %d", n)
	}
}

func TestTheTrackFileHoldsNoAddressOfAnotherPerson(t *testing.T) {
	p := filepath.Join(t.TempDir(), "radio.db")
	s, err := OpenStore(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(fakeVault{}, s)
	r.SetTrackHours(24)
	for i := 0; i < 3; i++ {
		cycleOf(t, r, t0.Add(time.Duration(i)*time.Minute), dataAt(neighbour, -60))
	}
	if n, _ := s.count("air_track"); n != 3 {
		t.Fatalf("points written: %d", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, f := range []string{p, p + "-wal"} {
		if b, err := os.ReadFile(f); err == nil {
			all = append(all, b...)
		}
	}
	for _, form := range [][]byte{neighbour[:], []byte(neighbour.String()), []byte(hex.EncodeToString(neighbour[:]))} {
		if bytes.Contains(all, form) {
			t.Errorf("the neighbour's address appears in the file as %q", form)
		}
	}
}

func TestNothingOutlivesItsAgeEvenWhenNothingIsListening(t *testing.T) {
	r := devRig(t)
	keep(t, r, "48")
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{0, 5}, []float64{-70, -60})
	if n, _ := store.count("air_track"); n != 2 {
		t.Fatalf("points written: %d", n)
	}
	r.mu.Lock()
	r.now = r.now.Add(49 * time.Hour) // no cycle runs again; only a read of the page
	r.mu.Unlock()
	var v KeepView
	getQ(t, r, KeepPath, "", &v)
	if n, _ := store.count("air_track"); n != 0 {
		t.Errorf("a point past the 48-hour ceiling is still on disk: %d", n)
	}
}

func TestChangingTheTrackFromFortyEightToTwentyFourPrunesAtOnce(t *testing.T) {
	r := devRig(t)
	keep(t, r, "48")
	rec, store, _ := r.s.recorder(r.s.h)
	cyclesAt(t, rec, r, []int{-30 * 60, 0}, []float64{-70, -60}) // one point 30 hours old, one now
	r.mu.Lock()
	r.now = r.now.Add(5 * time.Minute)
	r.mu.Unlock()
	keep(t, r, "24")
	if n, _ := store.count("air_track"); n != 1 {
		t.Errorf("only the point inside 24 hours stays: %d", n)
	}
}
