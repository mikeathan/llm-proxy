package capture

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// fakeHostSource hands out frames, one per tick of a fake clock, and remembers every sampling rate it was told.
type fakeHostSource struct {
	frames      [][]byte
	err         error
	setErr      error
	seen, drops uint32
	clock       *fakeClock
	tick        time.Duration
	rates       []uint16
}

func (f *fakeHostSource) Next() ([]byte, error) {
	f.clock.t = f.clock.t.Add(f.tick)
	if len(f.frames) == 0 {
		return nil, f.err
	}
	fr := f.frames[0]
	f.frames = f.frames[1:]
	return fr, nil
}
func (f *fakeHostSource) Stats() (uint32, uint32) { return f.seen, f.drops }
func (f *fakeHostSource) Close() error            { return nil }
func (f *fakeHostSource) SetSample(n uint16) error {
	f.rates = append(f.rates, n)
	return f.setErr
}

func runHost(t *testing.T, src *fakeHostSource, cfg HostConfig) ([]wire.HostRecord, wire.HostTrailer) {
	t.Helper()
	cfg.Now = src.clock.now
	var out bytes.Buffer
	if err := RunHost(src, &out, cfg); err != nil {
		t.Fatal(err)
	}
	recs, tr, err := wire.DecodeHostCapture(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return recs, tr
}

func hostSource(frames ...[]byte) *fakeHostSource {
	return &fakeHostSource{clock: &fakeClock{time.Unix(1000, 0)}, frames: frames, tick: 100 * time.Millisecond}
}

func defaultHostConfig(seconds int) HostConfig {
	return HostConfig{Seconds: seconds, MaxFrames: wire.HostMaxFrames, MaxOutput: wire.HostMaxOutput, BudgetPerSecond: hostBudgetPerSecond}
}

func TestRunHostStopsAtTheWindowWithEveryFrameAtRateOne(t *testing.T) {
	src := hostSource([]byte{1}, []byte{2})
	src.seen = 2
	recs, tr := runHost(t, src, defaultHostConfig(1))
	if tr.Stop != wire.StopWindow || tr.Frames != 2 || len(recs) != 2 || tr.KernelSeen != 2 || tr.Sample != 1 {
		t.Fatalf("stop=%d frames=%d records=%d sample=%d", tr.Stop, tr.Frames, len(recs), tr.Sample)
	}
	if recs[0].Sample != 1 || recs[1].Sample != 1 {
		t.Fatalf("a quiet window is not sampled: %+v", recs)
	}
	if len(src.rates) != 0 {
		t.Fatalf("the program changed the sampling rate of a quiet window: %v", src.rates)
	}
}

func TestRunHostStopsAtTheFrameCapAndSaysSo(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 10; i++ {
		frames = append(frames, []byte{byte(i)})
	}
	src := hostSource(frames...)
	src.drops = 3
	cfg := defaultHostConfig(60)
	cfg.MaxFrames = 4
	recs, tr := runHost(t, src, cfg)
	if tr.Stop != wire.StopFrameCap || len(recs) != 4 || tr.KernelDropped != 3 {
		t.Fatalf("stop=%d records=%d dropped=%d", tr.Stop, len(recs), tr.KernelDropped)
	}
}

func TestRunHostStopsBeforeTheOutputPassesItsCap(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 50; i++ {
		frames = append(frames, bytes.Repeat([]byte{7}, wire.HostSnapMax))
	}
	src := hostSource(frames...)
	cfg := defaultHostConfig(60)
	cfg.MaxOutput = 5*(wire.HostRecordHeader+wire.HostSnapMax) + wire.HostTrailerSize
	recs, tr := runHost(t, src, cfg)
	if tr.Stop != wire.StopFrameCap || len(recs) != 5 {
		t.Fatalf("stop=%d records=%d; the limit holds five full records and the trailer", tr.Stop, len(recs))
	}
}

func TestRunHostCutsAFrameToTheLongestRecord(t *testing.T) {
	src := hostSource(bytes.Repeat([]byte{7}, 900))
	recs, _ := runHost(t, src, defaultHostConfig(1))
	if len(recs[0].Frame) != wire.HostSnapMax || recs[0].OffsetMs == 0 {
		t.Fatalf("frame %d bytes at %d ms", len(recs[0].Frame), recs[0].OffsetMs)
	}
}

func TestRunHostDoublesTheSamplingRateWhenMoreThanTheBudgetArrivesInASecond(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 3000; i++ {
		frames = append(frames, []byte{1})
	}
	src := hostSource(frames...)
	src.tick = time.Millisecond // 1,000 frames a second
	cfg := defaultHostConfig(60)
	cfg.BudgetPerSecond = 100
	recs, tr := runHost(t, src, cfg)
	if len(src.rates) == 0 {
		t.Fatal("the rate never changed although the budget was passed many times over")
	}
	for i, r := range src.rates {
		if r != 1<<(i+1) {
			t.Fatalf("rates = %v, want each one double the last", src.rates)
		}
	}
	if recs[0].Sample != 1 || recs[len(recs)-1].Sample != tr.Sample || tr.Sample != src.rates[len(src.rates)-1] {
		t.Fatalf("records start at rate 1 and end at the last rate told to the source: first=%d last=%d trailer=%d rates=%v", recs[0].Sample, recs[len(recs)-1].Sample, tr.Sample, src.rates)
	}
	for i := 1; i < len(recs); i++ {
		if recs[i].Sample < recs[i-1].Sample {
			t.Fatalf("the sampling rate went down at record %d", i)
		}
	}
}

func TestRunHostNeverSamplesCoarserThanTheLimit(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 20000; i++ {
		frames = append(frames, []byte{1})
	}
	src := hostSource(frames...)
	src.tick = time.Millisecond // 1,000 frames a second against a budget of 2: the rate doubles every second
	cfg := defaultHostConfig(60)
	cfg.BudgetPerSecond = 2
	_, tr := runHost(t, src, cfg)
	if tr.Sample != wire.HostMaxSample {
		t.Fatalf("final rate %d, want the limit %d", tr.Sample, wire.HostMaxSample)
	}
	for _, r := range src.rates {
		if r > wire.HostMaxSample {
			t.Fatalf("asked for 1 in %d", r)
		}
	}
}

func TestRunHostAReadErrorEndsTheWindowWithAReasonNotAFailure(t *testing.T) {
	src := hostSource([]byte{1})
	src.err = errors.New("network is down")
	recs, tr := runHost(t, src, defaultHostConfig(30))
	if tr.Stop != wire.StopSocketError || len(recs) != 1 {
		t.Fatalf("stop=%d records=%d", tr.Stop, len(recs))
	}
}

func TestRunHostAFilterThatCannotBeChangedEndsTheWindowWithAReason(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 500; i++ {
		frames = append(frames, []byte{1})
	}
	src := hostSource(frames...)
	src.tick = time.Millisecond
	src.setErr = errors.New("kernel refused")
	cfg := defaultHostConfig(60)
	cfg.BudgetPerSecond = 10
	_, tr := runHost(t, src, cfg)
	if tr.Stop != wire.StopSocketError {
		t.Fatalf("stop=%d; a window that cannot keep to its budget must stop, not run unbounded", tr.Stop)
	}
}

func TestHostMainRefusesBadArgumentsAndNeverTakesALimitFromACaller(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--iface", "eth0"}, {"--iface", "eth0", "--seconds", "0"}, {"--iface", "eth0", "--seconds", "61"},
		{"--iface", "../etc", "--seconds", "5"}, {"--iface", "eth0", "--seconds", "5", "--max-frames", "9999999"}, {"--iface", "eth0", "--seconds", "5", "--sample", "1"}, {"--iface", "eth0", "--seconds", "5", "extra"},
	} {
		var out, errOut bytes.Buffer
		if code := HostMain(args, &out, &errOut); code != 2 || out.Len() != 0 {
			t.Errorf("%v: exit %d with %d bytes written", args, code, out.Len())
		}
	}
}
