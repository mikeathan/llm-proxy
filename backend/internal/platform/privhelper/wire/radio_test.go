package wire

import (
	"bytes"
	"errors"
	"testing"
)

// frame builds a radiotap-prefixed frame: a 12-byte radiotap header, then the frame control byte and filler.
func radioFrame(fc byte, total int) []byte {
	f := make([]byte, total)
	f[2] = 12 // radiotap length, little endian
	f[12] = fc
	return f
}

func TestRadioSnapKeepsABeaconLongerThanAnyOtherFrame(t *testing.T) {
	if got := RadioSnapFor(radioFrame(0x80, 700)); got != RadioSnapBeacon {
		t.Errorf("beacon kept %d", got)
	}
	for name, fc := range map[string]byte{"probe request": 0x40, "probe response": 0x50, "data": 0x08, "rts": 0xb4, "deauth": 0xc0} {
		if got := RadioSnapFor(radioFrame(fc, 700)); got != RadioSnapOther {
			t.Errorf("%s kept %d", name, got)
		}
	}
}

func TestRadioSnapOfAFrameThatCannotBeReadIsTheShortCut(t *testing.T) {
	for name, f := range map[string][]byte{
		"empty": nil, "three bytes": {0, 0, 12}, "radiotap length past the end": {0, 0, 200, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		"radiotap length too small": {0, 0, 2, 0, 0x80, 0, 0, 0, 0, 0, 0, 0}, "version not 0": append([]byte{1, 0, 12, 0}, make([]byte, 100)...),
	} {
		if got := RadioSnapFor(f); got > RadioSnapOther || got > len(f) {
			t.Errorf("%s: kept %d of %d", name, got, len(f))
		}
	}
}

func TestRadioRoundTrip(t *testing.T) {
	beacon, other := radioFrame(0x80, 900), radioFrame(0x40, 200)
	var out []byte
	out = AppendRadioRecord(out, 5, beacon)
	out = AppendRadioRecord(out, 90, other)
	out = AppendRadioTrailer(out, RadioTrailer{Stop: StopFrameCap, Frames: 2, KernelSeen: 9, KernelDropped: 1, ElapsedMs: 20000})
	recs, tr, err := DecodeRadio(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || len(recs[0].Frame) != RadioSnapBeacon || len(recs[1].Frame) != RadioSnapOther || recs[1].OffsetMs != 90 {
		t.Fatalf("records = %d, %d, %d", len(recs), len(recs[0].Frame), len(recs[1].Frame))
	}
	if !bytes.Equal(recs[0].Frame, beacon[:RadioSnapBeacon]) {
		t.Error("the kept bytes are the frame's first bytes")
	}
	if tr.Stop != StopFrameCap || tr.Frames != 2 || tr.KernelSeen != 9 || tr.KernelDropped != 1 || tr.ElapsedMs != 20000 {
		t.Fatalf("trailer = %+v", tr)
	}
}

func TestRadioACutRunKeepsTheWholeRecordsAndSaysSo(t *testing.T) {
	out := AppendRadioRecord(AppendRadioRecord(nil, 1, radioFrame(0x40, 100)), 2, radioFrame(0x40, 100))
	recs, tr, err := DecodeRadio(out[:len(out)-10])
	if err != nil || len(recs) != 1 || tr.Stop != StopCut || tr.Frames != 1 {
		t.Fatalf("%d records, %+v, %v", len(recs), tr, err)
	}
}

func TestRadioDecodeRefusesWhatDisagrees(t *testing.T) {
	good := AppendRadioRecord(nil, 1, radioFrame(0x40, 100))
	cases := map[string][]byte{
		"trailer counts another number": AppendRadioTrailer(slicesClone(good), RadioTrailer{Frames: 2}),
		"record longer than the limit":  append([]byte{0, 0, 0, 1, 0x03, 0xe8, 0, 0}, make([]byte, 1000)...),
		"unknown stop reason":           AppendRadioTrailer(slicesClone(good), RadioTrailer{Stop: StopUnsupported + 1, Frames: 1}),
		"trailer after a cut record":    AppendRadioTrailer(append(slicesClone(good), 0, 0, 0, 1, 0, 60, 0, 0), RadioTrailer{Frames: 1}),
	}
	for name, data := range cases {
		if _, _, err := DecodeRadio(data); !errors.Is(err, ErrRadioFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func slicesClone(b []byte) []byte { return append([]byte(nil), b...) }

func FuzzDecodeRadio(f *testing.F) {
	f.Add(AppendRadioTrailer(AppendRadioRecord(nil, 1, radioFrame(0x80, 600)), RadioTrailer{Frames: 1}))
	f.Fuzz(func(t *testing.T, data []byte) {
		recs, _, err := DecodeRadio(data)
		if err == nil && len(recs) > RadioMaxFrames {
			t.Fatal("more records than a window may keep")
		}
		for _, r := range recs {
			if len(r.Frame) > RadioSnapBeacon {
				t.Fatal("a record longer than the limit")
			}
		}
	})
}
