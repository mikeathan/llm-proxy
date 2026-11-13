package capture

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

func radioFrame(fc byte, total int) []byte {
	f := make([]byte, total)
	f[2] = 12 // radiotap length
	f[12] = fc
	return f
}

func runRadio(t *testing.T, src *fakeHostSource, seconds, maxFrames int) ([]wire.RadioRecord, wire.RadioTrailer) {
	t.Helper()
	var out bytes.Buffer
	if err := RunRadio(src, &out, RadioConfig{Seconds: seconds, MaxFrames: maxFrames, Now: src.clock.now}); err != nil {
		t.Fatal(err)
	}
	recs, tr, err := wire.DecodeRadio(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return recs, tr
}

func TestRunRadioKeepsABeaconLongerThanOtherFramesAndStopsAtTheWindow(t *testing.T) {
	src := hostSource(radioFrame(0x80, 700), radioFrame(0x40, 300), radioFrame(0xb4, 40))
	recs, tr := runRadio(t, src, 2, wire.RadioMaxFrames)
	if tr.Stop != wire.StopWindow || len(recs) != 3 {
		t.Fatalf("stop=%d records=%d", tr.Stop, len(recs))
	}
	if len(recs[0].Frame) != wire.RadioSnapBeacon || len(recs[1].Frame) != wire.RadioSnapOther || len(recs[2].Frame) != 40 {
		t.Errorf("kept %d, %d, %d", len(recs[0].Frame), len(recs[1].Frame), len(recs[2].Frame))
	}
	if tr.ElapsedMs < 2000 {
		t.Errorf("the window ran %d ms, want at least 2000", tr.ElapsedMs)
	}
}

func TestRunRadioStopsAtTheFrameCapAndSaysSo(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 50; i++ {
		frames = append(frames, radioFrame(0x40, 100))
	}
	recs, tr := runRadio(t, hostSource(frames...), 30, 10)
	if tr.Stop != wire.StopFrameCap || len(recs) != 10 || tr.Frames != 10 {
		t.Fatalf("stop=%d records=%d", tr.Stop, len(recs))
	}
}

func TestRunRadioAReadErrorEndsTheWindowWithAReasonNotAFailure(t *testing.T) {
	src := hostSource(radioFrame(0x40, 100))
	src.err = errors.New("network is down")
	recs, tr := runRadio(t, src, 20, wire.RadioMaxFrames)
	if tr.Stop != wire.StopSocketError || len(recs) != 1 {
		t.Fatalf("stop=%d records=%d", tr.Stop, len(recs))
	}
}

func TestRunRadioTheLargestPossibleOutputFitsTheCatalogueCap(t *testing.T) {
	var frames [][]byte
	for i := 0; i < wire.RadioMaxFrames; i++ {
		frames = append(frames, radioFrame(0x80, 900))
	}
	src := hostSource(frames...)
	src.tick = time.Millisecond
	var out bytes.Buffer
	if err := RunRadio(src, &out, RadioConfig{Seconds: wire.RadioMaxSeconds, MaxFrames: wire.RadioMaxFrames, Now: src.clock.now}); err != nil {
		t.Fatal(err)
	}
	if out.Len() > wire.RadioMaxOutput {
		t.Fatalf("%d bytes, cap %d", out.Len(), wire.RadioMaxOutput)
	}
}

func TestRadioMainRefusesWhatItCouldNotOpenWithATrailerAndArgumentsItDoesNotUnderstandWithStatus2(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--iface", "../x", "--seconds", "5"}, {"--iface", "wlan1", "--seconds", "31"}, {"--iface", "wlan1", "--seconds", "0"},
		{"--iface", "wlan1", "--seconds", "5", "extra"}, {"--bogus"},
	} {
		var out, errOut bytes.Buffer
		if code := RadioMain(args, &out, &errOut); code != 2 || out.Len() != 0 {
			t.Errorf("%v: status %d, wrote %d bytes", args, code, out.Len())
		}
	}
}
