package radio

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
)

// A radio_listen output is built here the way the helper writes it (the layout is stated in wire/radio.go; the core's contract test builds it
// with that package and decodes it with this one, so the two copies of the layout cannot drift).
func windowBytes(stop byte, frames [][]byte, trailer bool) []byte {
	var out []byte
	for i, f := range frames {
		var h [8]byte
		binary.BigEndian.PutUint32(h[0:4], uint32(i*100))
		binary.BigEndian.PutUint16(h[4:6], uint16(len(f)))
		out = append(append(out, h[:]...), f...)
	}
	if trailer {
		var tr [32]byte
		copy(tr[0:4], "RWT1")
		tr[4] = stop
		binary.BigEndian.PutUint32(tr[8:12], uint32(len(frames)))
		binary.BigEndian.PutUint32(tr[12:16], 90)
		binary.BigEndian.PutUint32(tr[16:20], 3)
		binary.BigEndian.PutUint32(tr[20:24], 20000)
		out = append(out, tr[:]...)
	}
	return out
}

func TestDecodeWindowReadsRecordsAndTrailer(t *testing.T) {
	w, err := DecodeWindow(windowBytes(0, [][]byte{data(neighbour), beacon("home")}, true))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Records) != 2 || w.Trailer.Stop != StopWindow || w.Trailer.KernelSeen != 90 || w.Trailer.KernelDropped != 3 || w.Trailer.ElapsedMs != 20000 || !w.Trailer.Present {
		t.Errorf("%+v", w)
	}
	if w.Records[1].OffsetMs != 100 {
		t.Errorf("offset %d", w.Records[1].OffsetMs)
	}
}

func TestDecodeWindowWithoutATrailerIsACutRun(t *testing.T) {
	w, err := DecodeWindow(windowBytes(0, [][]byte{data(neighbour)}, false))
	if err != nil || w.Trailer.Stop != StopCut || w.Trailer.Present || len(w.Records) != 1 {
		t.Errorf("%+v %v", w, err)
	}
	// A record cut in the middle is dropped, not read.
	cut := windowBytes(0, [][]byte{data(neighbour), data(neighbour)}, false)
	w, err = DecodeWindow(cut[:len(cut)-5])
	if err != nil || len(w.Records) != 1 || w.Trailer.Stop != StopCut {
		t.Errorf("%+v %v", w, err)
	}
}

func TestDecodeWindowRefusesWhatTheHelperNeverWrites(t *testing.T) {
	long := make([]byte, maxKept+1)
	frames := make([][]byte, maxFrames+1)
	for i := range frames {
		frames[i] = []byte{0}
	}
	bad := map[string][]byte{
		"a record over the limit":       windowBytes(0, [][]byte{long}, true),
		"more records than a window":    windowBytes(0, frames, false),
		"a trailer that miscounts":      append(windowBytes(0, [][]byte{data(neighbour)}, false), windowBytes(0, [][]byte{data(neighbour), data(neighbour)}, true)[2*(8+len(data(neighbour))):]...),
		"an unknown stop reason":        windowBytes(99, nil, true),
		"a record cut before a trailer": append(windowBytes(0, [][]byte{data(neighbour)}, false)[:20], windowBytes(0, nil, true)...),
	}
	for name, b := range bad {
		if _, err := DecodeWindow(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func FuzzDecodeWindow(f *testing.F) {
	f.Add(windowBytes(0, [][]byte{data(neighbour), beacon("x")}, true))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if w, err := DecodeWindow(b); err == nil && len(w.Records) > maxFrames {
			t.Fatal("too many records")
		}
	})
}

func TestIngestRecordsFramesAndSummarisesTheWindow(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	raw := windowBytes(0, [][]byte{data(neighbour), beacon("home"), {1, 2, 3}}, true)
	sum, err := Ingest(context.Background(), r, raw, WindowAsked{Channel: 60}, t0)
	if err != nil {
		t.Fatal(err)
	}
	// data(): sender recorded; beacon: AP recorded; garbage: counted as unread, not an error.
	if sum.Frames != 3 || sum.Recorded != 2 || sum.Unread != 1 || sum.Stop != "window" || sum.KernelDropped != 3 {
		t.Errorf("%+v", sum)
	}
	if sum.OnChannel != 2 || sum.OffChannel != 0 || !sum.ChannelMatched {
		t.Errorf("the radiotap frequency is 5300 MHz, channel 60: %+v", sum)
	}
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, neighbour[:])
	if _, ok, _ := s.Device(context.Background(), tag); !ok {
		t.Error("the sender was not recorded")
	}
}

func TestIngestSaysWhenTheAdapterWasOnAnotherChannel(t *testing.T) {
	s, _ := openTest(t, 0)
	sum, err := Ingest(context.Background(), NewRecorder(fakeVault{}, s), windowBytes(0, [][]byte{data(neighbour)}, true), WindowAsked{Channel: 1}, t0)
	if err != nil || sum.ChannelMatched || sum.OffChannel != 1 || sum.HeardChannels[0] != 60 {
		t.Errorf("the read-back disagrees with the frames and the summary must say so: %+v %v", sum, err)
	}
}

func TestIngestAWindowThatReadNothingNamesWhy(t *testing.T) {
	s, _ := openTest(t, 0)
	sum, err := Ingest(context.Background(), NewRecorder(fakeVault{}, s), windowBytes(byte(StopInterfaceDown), nil, true), WindowAsked{Channel: 60}, time.Time{})
	if err != nil || sum.Stop != "interface_down" || sum.Frames != 0 || sum.ChannelMatched {
		t.Errorf("%+v %v", sum, err)
	}
}
