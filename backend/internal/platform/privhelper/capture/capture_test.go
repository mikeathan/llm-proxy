package capture

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// fakeSource hands out frames in order; after them it reports "nothing arrived" until the clock passes the window.
type fakeSource struct {
	frames      [][]byte
	err         error
	seen, drops uint32
	clock       *fakeClock
}

func (f *fakeSource) Next() ([]byte, error) {
	f.clock.t = f.clock.t.Add(100 * time.Millisecond)
	if len(f.frames) == 0 {
		return nil, f.err
	}
	fr := f.frames[0]
	f.frames = f.frames[1:]
	return fr, nil
}
func (f *fakeSource) Stats() (uint32, uint32) { return f.seen, f.drops }
func (f *fakeSource) Close() error            { return nil }

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func run(t *testing.T, src *fakeSource, seconds, max int) ([]wire.CaptureRecord, wire.CaptureTrailer) {
	t.Helper()
	var out bytes.Buffer
	if err := Run(src, &out, Config{Seconds: seconds, MaxFrames: max, Now: src.clock.now}); err != nil {
		t.Fatal(err)
	}
	recs, tr, err := wire.DecodeCapture(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return recs, tr
}

func TestRunStopsAtTheWindow(t *testing.T) {
	src := &fakeSource{clock: &fakeClock{time.Unix(1000, 0)}, frames: [][]byte{{1}, {2}}, seen: 2}
	recs, tr := run(t, src, 1, 100)
	if tr.Stop != wire.StopWindow || tr.Frames != 2 || len(recs) != 2 || tr.KernelSeen != 2 {
		t.Fatalf("stop=%d frames=%d records=%d", tr.Stop, tr.Frames, len(recs))
	}
	if tr.ElapsedMs < 1000 {
		t.Fatalf("a one-second window ended after %d ms", tr.ElapsedMs)
	}
}

func TestRunStopsAtTheFrameCapAndSaysSo(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 10; i++ {
		frames = append(frames, []byte{byte(i)})
	}
	src := &fakeSource{clock: &fakeClock{time.Unix(1000, 0)}, frames: frames, drops: 3}
	recs, tr := run(t, src, 60, 4)
	if tr.Stop != wire.StopFrameCap || len(recs) != 4 || tr.KernelDropped != 3 {
		t.Fatalf("stop=%d records=%d dropped=%d", tr.Stop, len(recs), tr.KernelDropped)
	}
}

func TestARecordCarriesWhenItArrivedAndAtMost64Bytes(t *testing.T) {
	src := &fakeSource{clock: &fakeClock{time.Unix(1000, 0)}, frames: [][]byte{bytes.Repeat([]byte{7}, 300)}}
	recs, _ := run(t, src, 1, 10)
	if len(recs[0].Frame) != wire.CaptureSnap || recs[0].OffsetMs == 0 {
		t.Fatalf("frame %d bytes at %d ms", len(recs[0].Frame), recs[0].OffsetMs)
	}
}

func TestAReadErrorEndsTheWindowWithAReasonNotAFailure(t *testing.T) {
	src := &fakeSource{clock: &fakeClock{time.Unix(1000, 0)}, frames: [][]byte{{1}}, err: errors.New("network is down")}
	recs, tr := run(t, src, 30, 10)
	if tr.Stop != wire.StopSocketError || len(recs) != 1 {
		t.Fatalf("stop=%d records=%d", tr.Stop, len(recs))
	}
}

func TestMainRefusesBadArgumentsAndNeverTakesTheFrameCapFromACaller(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--iface", "eth0"}, {"--iface", "eth0", "--seconds", "0"}, {"--iface", "eth0", "--seconds", "61"},
		{"--iface", "../etc", "--seconds", "5"}, {"--iface", "eth0", "--seconds", "5", "--max-frames", "9999999"}, {"--iface", "eth0", "--seconds", "5", "extra"},
	} {
		var out, errOut bytes.Buffer
		if code := Main(args, &out, &errOut); code != 2 || out.Len() != 0 {
			t.Errorf("%v: exit %d with %d bytes written", args, code, out.Len())
		}
	}
}

func TestInterfaceNamesAreChecked(t *testing.T) {
	for name, want := range map[string]bool{"wlp2s0": true, "eth0.100": true, "br-lan": true, "": false, "a b": false, "a/b": false, "-x": false, "0123456789abcdef": false, "..": false} {
		if got := ValidInterface(name); got != want {
			t.Errorf("%q: got %v", name, got)
		}
	}
}
