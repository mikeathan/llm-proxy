package radio

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// focusWindow is one focus window's bytes: frames on channel 60 (the fixtures' radiotap frequency).
func focusWindow(frames ...[]byte) []byte { return windowBytes(0, frames, true) }

func storedState(t *testing.T, s *Store) ([]FeatureRow, map[int64]int) {
	t.Helper()
	rows, listened, err := s.FeatureRows(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return rows, listened
}

// A focus window moves the live picture and writes nothing: the stored rows after sweeps with focus windows between them are exactly the rows of
// the sweeps alone, so no share, class, movement count or track changes because the radio listened more.
func TestAFocusWindowMovesTheDotAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	sweepsOnly, _ := openTest(t, 0)
	withFocus, _ := openTest(t, 0)
	a, b := NewRecorder(fakeVault{}, sweepsOnly), NewRecorder(fakeVault{}, withFocus)
	for _, r := range []*Recorder{a, b} {
		if err := r.AddOwnerDevice(ctx, neighbour, "Phone", t0); err != nil {
			t.Fatal(err)
		}
		r.SetTrackHours(24) // the owner's track on, so a focus window that wrote a point would show
	}
	at := t0
	for i := range 3 {
		cycleOf(t, a, at, dataAt(neighbour, -70), dataAt(mineMAC, -80))
		cycleOf(t, b, at, dataAt(neighbour, -70), dataAt(mineMAC, -80))
		if i == 1 {
			if _, err := FocusIngest(ctx, b, focusWindow(dataAt(neighbour, -48), dataAt(mineMAC, -50), beacon("home")), WindowAsked{Channel: 60}, at.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			s := sampleOf(t, b, neighbour)
			if s.signal != -48 || !s.heard || !s.moving {
				t.Errorf("the focus reading is the dot's newest, and a 22 dB jump is movement: %+v", s)
			}
		}
		at = at.Add(5 * time.Minute)
	}
	rowsA, listenedA := storedState(t, sweepsOnly)
	rowsB, listenedB := storedState(t, withFocus)
	if !reflect.DeepEqual(rowsA, rowsB) || !reflect.DeepEqual(listenedA, listenedB) {
		t.Errorf("a focus window must change nothing stored:\nsweeps only %+v\nwith focus  %+v", rowsA, rowsB)
	}
	for _, m := range []MAC{neighbour, mineMAC} {
		pa, errA := sweepsOnly.TrackPoints(ctx, mustTag(t, a, m), t0)
		pb, errB := withFocus.TrackPoints(ctx, mustTag(t, b, m), t0)
		if errA != nil || errB != nil || !reflect.DeepEqual(pa, pb) {
			t.Errorf("a focus window writes no track point for %v: %v %v (%v %v)", m, pa, pb, errA, errB)
		}
	}
}

func sampleOf(t *testing.T, r *Recorder, m MAC) liveSample {
	t.Helper()
	tag, _ := r.tag(context.Background(), tagDevice, m[:])
	for _, s := range r.LiveSnapshot() {
		if string(s.tag) == string(tag) {
			return s
		}
	}
	t.Fatalf("%v is not in the picture", m)
	return liveSample{}
}

// A device heard only in a focus window since the last sweep is in the picture; its readings are bounded and forgotten with the run.
func TestFocusReadingsAreBoundedAndForgottenWithTheRun(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	cycleOf(t, r, t0, dataAt(mineMAC, -80))
	for i := range focusReadings + 5 {
		if _, err := FocusIngest(context.Background(), r, focusWindow(dataAt(neighbour, -60)), WindowAsked{Channel: 60}, t0.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := sampleOf(t, r, neighbour); len(got.trail) != focusReadings || !got.heard {
		t.Errorf("a device keeps at most %d focus readings: %+v", focusReadings, got)
	}
	if _, err := FocusIngest(context.Background(), r, focusWindow(dataAt(mineMAC, -60)), WindowAsked{Channel: 60}, t0.Add(focusKeepFor+time.Hour)); err != nil {
		t.Fatal(err)
	}
	r.cmu.Lock()
	_, old := r.focus[string(mustTag(t, r, neighbour))]
	r.cmu.Unlock()
	if old {
		t.Error("a reading older than focusKeepFor is forgotten")
	}
	r.ClearLive()
	if r.focus != nil {
		t.Error("a run that ends leaves no focus reading behind")
	}
}

func mustTag(t *testing.T, r *Recorder, m MAC) []byte {
	t.Helper()
	tag, err := r.tag(context.Background(), tagDevice, m[:])
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func TestAFocusWindowOnTheWrongChannelOrWithoutASignalFollowsNobody(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	sum, err := FocusIngest(context.Background(), r, focusWindow(dataAt(neighbour, -60)), WindowAsked{Channel: 6}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.OffChannel != 1 || sum.ChannelMatched || len(r.LiveSnapshot()) != 0 {
		t.Errorf("a frame from channel 60 in a channel-6 window is not a reading: %+v", sum)
	}
}

// The focus windows go to the channels the owner's devices were heard on, in proportion to their frames, and never to a channel with none.
func TestFocusFollowsTheOwnersChannels(t *testing.T) {
	p := newFocusPlan([]WindowSummary{
		{Channel: 1, Frames: 500, OwnFrames: 0, ChannelMatched: true},
		{Channel: 6, Frames: 3300, OwnFrames: 700, ChannelMatched: true},
		{Channel: 60, Frames: 4000, OwnFrames: 1400, ChannelMatched: true},
		{Channel: 36, Frames: 600, OwnFrames: 900, ChannelMatched: false}, // frames from another channel: not evidence for 36
	})
	got := map[int]int{}
	for range 30 {
		ch, ok := p.next()
		if !ok {
			t.Fatal("two channels earn focus")
		}
		got[ch]++
	}
	if got[60] != 20 || got[6] != 10 || got[1] != 0 || got[36] != 0 {
		t.Errorf("two thirds of the owner's frames were on 60: %v", got)
	}
	if p.frames[60] != 4000 {
		t.Errorf("a focus window is expected to read what the sweep's did: %v", p.frames)
	}
	if _, ok := newFocusPlan([]WindowSummary{{Channel: 1, Frames: 500, ChannelMatched: true}}).next(); ok {
		t.Error("no owner's frames, no focus: the sweep already visits every channel")
	}
}

func TestFocusKeepsToItsBudgetAndThePlanToTheHelpersCap(t *testing.T) {
	b := focusBudget{started: t0}
	if !b.allows(t0, RadioMaxWindowFrames) {
		t.Error("the first window is allowed on an empty clock")
	}
	b.frames = RadioMaxWindowFrames
	if b.allows(t0, 1) || !b.allows(t0.Add(time.Hour), focusFramesPerHour) {
		t.Errorf("frames come at the budget's pace: %+v", b)
	}
	def := Settings{}.Plan()
	if def.focusPerHour() != 72 || def.sessionWindowsPerHour() != 144 {
		t.Errorf("the default plan: 72 sweep windows an hour leave room for 72 focus windows: %d %d", def.focusPerHour(), def.sessionWindowsPerHour())
	}
	for _, set := range []Settings{{}, {CycleSeconds: 60}, {WindowSeconds: 30, CycleSeconds: 120}, {Channels: []int{6}, WindowSeconds: 5, CycleSeconds: 60}, {CycleSeconds: 3600}} {
		p := set.Plan()
		if n := p.sessionWindowsPerHour(); n > MaxWindowsPerHour || p.focusPerHour() < 0 {
			t.Errorf("%+v asks %d windows an hour, more than %d", p, n, MaxWindowsPerHour)
		}
	}
}

// In a run, a focus window is one of the requests the session approved (a channel of the plan at the plan's length), and a refused one turns
// focusing off for the run without ending it.
func TestARunFocusesOnTheOwnersChannelAndARefusalNeverEndsTheWatch(t *testing.T) {
	r := newRig(t, twoChannels, func(n int, _ ListenParams) (stacksdk.PrivilegedResult, error) {
		if n == 6 { // the second cycle's focus window
			return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeDenied, "the session's uses are spent")
		}
		return stacksdk.PrivilegedResult{Kind: "bytes", Data: windowBytes(0, [][]byte{dataAt(neighbour, -60)}, true)}, nil
	})
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Phone", r.now); err != nil {
		t.Fatal(err)
	}
	if resp := r.post(t, `{"adapter":"wlx00","hours":1,"focus":true}`); resp.Status != http.StatusAccepted {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	waitFor(t, "the sweep and one focus window", func() bool { return r.priv.count() == 3 && len(r.nap) == 1 })
	r.priv.mu.Lock()
	focus := r.priv.asked[2]
	r.priv.mu.Unlock()
	if focus != (ListenParams{Iface: "wlx00", Channel: 60, Seconds: 20}) {
		t.Errorf("the focus window is the plan's own channel-60 request: %+v", focus)
	}
	if st := r.state(t); st.FocusWindows != 1 || st.FocusFrames != 1 || st.Frames != 2 {
		t.Errorf("focus frames are counted apart from the sweep's: %+v", st)
	}
	<-r.nap
	r.step <- struct{}{}
	waitFor(t, "the next sweep, then a refused focus window", func() bool { return r.priv.count() == 6 && len(r.nap) == 1 })
	st := r.state(t)
	if !st.Listening || !st.FocusOff || st.Ended != "" {
		t.Errorf("a refused focus window turns focus off and the watch goes on: %+v", st)
	}
	<-r.nap
	r.step <- struct{}{}
	waitFor(t, "a sweep with no focus window", func() bool { return r.priv.count() == 8 && len(r.nap) == 1 })
}

// benchWindow is a full window (RadioMaxWindowFrames) from 60 devices: what channel 60 returns on vertex.
func benchWindow() []byte {
	frames := make([][]byte, RadioMaxWindowFrames)
	for i := range frames {
		m := neighbour
		m[5] = byte(i % 60)
		frames[i] = dataAt(m, -40-i%40)
	}
	return windowBytes(0, frames, true)
}

// BenchmarkFocusIngest is one full focus window: parse, tag and sum, no database.
func BenchmarkFocusIngest(b *testing.B) {
	s, _ := openTest(b, 0)
	r := NewRecorder(fakeVault{}, s)
	raw := benchWindow()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FocusIngest(context.Background(), r, raw, WindowAsked{Channel: 60}, t0); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*RadioMaxWindowFrames), "ns/frame")
}

// BenchmarkSweepIngest is the same window through the sweep's path, inside an open cycle (the first frame of each device is written).
func BenchmarkSweepIngest(b *testing.B) {
	s, _ := openTest(b, 0)
	r := NewRecorder(fakeVault{}, s)
	raw := benchWindow()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.BeginCycle()
		if _, err := Ingest(context.Background(), r, raw, WindowAsked{Channel: 60}, t0); err != nil {
			b.Fatal(err)
		}
		r.DropCycle()
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*RadioMaxWindowFrames), "ns/frame")
}

// A start that did not ask for focus windows (a session sized for sweeps only) never runs one: a session's uses are one pool, and focus
// windows on it would spend the sweeps' and end the watch early.
func TestAStartThatDidNotAskForFocusOnlySweeps(t *testing.T) {
	r := newRig(t, twoChannels, func(int, ListenParams) (stacksdk.PrivilegedResult, error) {
		return stacksdk.PrivilegedResult{Kind: "bytes", Data: windowBytes(0, [][]byte{dataAt(neighbour, -60)}, true)}, nil
	})
	rec, _, _ := r.s.recorder(r.s.h)
	if err := rec.AddOwnerDevice(context.Background(), neighbour, "Phone", r.now); err != nil {
		t.Fatal(err)
	}
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "the sweep, then the wait", func() bool { return r.priv.count() == 2 && len(r.nap) == 1 })
	if st := r.state(t); st.FocusWindows != 0 {
		t.Errorf("no focus window without the start asking: %+v", st)
	}
}
