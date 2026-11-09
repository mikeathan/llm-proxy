package wire

import (
	"bytes"
	"errors"
	"testing"
)

func TestHostCaptureRoundTrip(t *testing.T) {
	var out []byte
	dns := bytes.Repeat([]byte{0xcd}, 300)
	long := bytes.Repeat([]byte{0xab}, HostSnapMax+100)
	out = AppendHostRecord(out, 5, 1, []byte{1, 2, 3})
	out = AppendHostRecord(out, 1500, 8, dns)
	out = AppendHostRecord(out, 1600, 1, long)
	out = AppendHostTrailer(out, HostTrailer{Stop: StopFrameCap, Frames: 3, KernelSeen: 9, KernelDropped: 1, ElapsedMs: 4000, Sample: 8})
	recs, tr, err := DecodeHostCapture(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].OffsetMs != 5 || recs[0].Sample != 1 || !bytes.Equal(recs[0].Frame, []byte{1, 2, 3}) {
		t.Fatalf("records = %+v", recs)
	}
	if recs[1].Sample != 8 || !bytes.Equal(recs[1].Frame, dns) {
		t.Fatalf("a long frame keeps its bytes up to the limit and its sampling rate: %+v", recs[1])
	}
	if len(recs[2].Frame) != HostSnapMax {
		t.Fatalf("a frame is cut to %d bytes, got %d", HostSnapMax, len(recs[2].Frame))
	}
	if tr.Stop != StopFrameCap || tr.Frames != 3 || tr.KernelSeen != 9 || tr.KernelDropped != 1 || tr.ElapsedMs != 4000 || tr.Sample != 8 {
		t.Fatalf("trailer = %+v", tr)
	}
}

func TestHostCaptureRecordsAreAsLongAsTheirFramesAndNoLonger(t *testing.T) {
	if got := len(AppendHostRecord(nil, 0, 1, make([]byte, 128))); got != HostRecordHeader+128 {
		t.Fatalf("a 128-byte frame took %d bytes", got)
	}
	if got := len(AppendHostRecord(nil, 0, 1, nil)); got != HostRecordHeader {
		t.Fatalf("an empty frame took %d bytes", got)
	}
}

func TestHostCaptureWithoutATrailerIsCutAndKeepsWholeRecords(t *testing.T) {
	out := AppendHostRecord(nil, 1, 1, []byte{9, 9})
	out = AppendHostRecord(out, 2, 1, []byte{8, 8, 8, 8})
	out = out[:len(out)-2] // the second record is cut short
	recs, tr, err := DecodeHostCapture(out)
	if err != nil || len(recs) != 1 || tr.Stop != StopCut || tr.Frames != 1 {
		t.Fatalf("recs=%d trailer=%+v err=%v", len(recs), tr, err)
	}
}

func TestHostCaptureRefusesWhatDoesNotAgree(t *testing.T) {
	good := AppendHostRecord(nil, 1, 1, []byte{1})
	t.Run("the trailer counts a different number of records", func(t *testing.T) {
		out := AppendHostTrailer(append([]byte(nil), good...), HostTrailer{Frames: 2})
		if _, _, err := DecodeHostCapture(out); !errors.Is(err, ErrHostCaptureFormat) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a record claims a frame longer than the limit", func(t *testing.T) {
		bad := AppendHostRecord(nil, 1, 1, []byte{1})
		bad[4], bad[5] = 0xff, 0xff
		out := AppendHostTrailer(bad, HostTrailer{Frames: 1})
		if _, _, err := DecodeHostCapture(out); !errors.Is(err, ErrHostCaptureFormat) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a record claims a sampling rate of zero or past the limit", func(t *testing.T) {
		for _, n := range []uint16{0, HostMaxSample + 1} {
			out := AppendHostTrailer(AppendHostRecord(nil, 1, n, []byte{1}), HostTrailer{Frames: 1})
			if _, _, err := DecodeHostCapture(out); !errors.Is(err, ErrHostCaptureFormat) {
				t.Fatalf("sample %d: err = %v", n, err)
			}
		}
	})
	t.Run("an unknown stop reason", func(t *testing.T) {
		out := AppendHostTrailer(nil, HostTrailer{Stop: StopUnsupported + 3})
		if _, _, err := DecodeHostCapture(out); !errors.Is(err, ErrHostCaptureFormat) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("more records than a window may keep", func(t *testing.T) {
		var out []byte
		for i := 0; i <= HostMaxFrames; i++ {
			out = AppendHostRecord(out, 0, 1, nil)
		}
		if _, _, err := DecodeHostCapture(out); !errors.Is(err, ErrHostCaptureFormat) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestHostCaptureTheLargestOutputFitsTheCap(t *testing.T) {
	if HostMaxOutput != 8<<20 {
		t.Fatalf("HostMaxOutput = %d, want 8 MiB (Constitution I.13)", HostMaxOutput)
	}
	if HostMaxFrames != 60000 {
		t.Fatalf("HostMaxFrames = %d, want 60,000 (Constitution I.13)", HostMaxFrames)
	}
}

// The response is base64 in JSON: the largest window must still fit the one frame the helper writes it in.
func TestHostCaptureAnOutputOfTheCapFitsAResponseFrame(t *testing.T) {
	if HostMaxOutput > MaxResponseBytes/4*3-4096 {
		t.Fatalf("a base64 body of %d bytes does not fit a response of %d", HostMaxOutput, MaxResponseBytes)
	}
}
