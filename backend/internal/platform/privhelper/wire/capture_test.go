package wire

import (
	"bytes"
	"errors"
	"testing"
)

func TestCaptureRoundTrip(t *testing.T) {
	var out []byte
	long := bytes.Repeat([]byte{0xab}, 200)
	out = AppendCaptureRecord(out, 5, []byte{1, 2, 3})
	out = AppendCaptureRecord(out, 1500, long)
	out = AppendCaptureTrailer(out, CaptureTrailer{Stop: StopFrameCap, Frames: 2, KernelSeen: 9, KernelDropped: 1, ElapsedMs: 4000})
	recs, tr, err := DecodeCapture(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].OffsetMs != 5 || !bytes.Equal(recs[0].Frame, []byte{1, 2, 3}) {
		t.Fatalf("records = %+v", recs)
	}
	if len(recs[1].Frame) != CaptureSnap {
		t.Fatalf("a frame is cut to %d bytes, got %d", CaptureSnap, len(recs[1].Frame))
	}
	if tr.Stop != StopFrameCap || tr.Frames != 2 || tr.KernelSeen != 9 || tr.KernelDropped != 1 || tr.ElapsedMs != 4000 {
		t.Fatalf("trailer = %+v", tr)
	}
}

func TestCaptureWithoutATrailerIsCutAndKeepsWholeRecords(t *testing.T) {
	out := AppendCaptureRecord(nil, 1, []byte{9})
	out = AppendCaptureRecord(out, 2, []byte{8})
	out = out[:len(out)-10] // the second record is cut
	recs, tr, err := DecodeCapture(out)
	if err != nil || len(recs) != 1 || tr.Stop != StopCut || tr.Frames != 1 {
		t.Fatalf("recs=%d trailer=%+v err=%v", len(recs), tr, err)
	}
}

func TestCaptureRefusesWhatDoesNotAgree(t *testing.T) {
	good := AppendCaptureTrailer(AppendCaptureRecord(nil, 1, []byte{1}), CaptureTrailer{Frames: 1})
	bad := map[string][]byte{
		"trailer says 2 frames":  AppendCaptureTrailer(AppendCaptureRecord(nil, 1, []byte{1}), CaptureTrailer{Frames: 2}),
		"unknown stop":           AppendCaptureTrailer(nil, CaptureTrailer{Stop: StopCut}),
		"records not a multiple": append(append([]byte{0}, good[:len(good)-CaptureTrailerSize]...), good[len(good)-CaptureTrailerSize:]...),
	}
	for name, data := range bad {
		if _, _, err := DecodeCapture(data); !errors.Is(err, ErrCaptureFormat) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// A record whose kept length is past the snap is refused.
	rec := AppendCaptureRecord(nil, 0, []byte{1})
	rec[5] = 200
	if _, _, err := DecodeCapture(AppendCaptureTrailer(rec, CaptureTrailer{Frames: 1})); !errors.Is(err, ErrCaptureFormat) {
		t.Errorf("long length: err = %v", err)
	}
}

func TestCaptureSizesAreConsistent(t *testing.T) {
	if CaptureRecordSize != 72 || CaptureMaxOutput != 4096*72+32 {
		t.Fatalf("record %d, max output %d", CaptureRecordSize, CaptureMaxOutput)
	}
	if CaptureMaxOutput > MaxResponseBytes/2 {
		t.Fatalf("a base64 body of %d bytes must fit a response of %d", CaptureMaxOutput, MaxResponseBytes)
	}
}
