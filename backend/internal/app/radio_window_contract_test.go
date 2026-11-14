package app

import (
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

// The helper writes a radio_listen window (wire/radio.go) and the Radio stack reads it with its own strict reader, because a stack cannot import
// the core. This builds a window with the helper's writer and reads it with the stack's reader, so the two copies of the layout cannot drift.
func TestRadioWindowWrittenByTheHelperIsReadByTheStack(t *testing.T) {
	frame := make([]byte, 40)
	var out []byte
	out = wire.AppendRadioRecord(out, 5, frame)
	out = wire.AppendRadioRecord(out, 900, frame)
	out = wire.AppendRadioTrailer(out, wire.RadioTrailer{Stop: wire.StopFrameCap, Frames: 2, KernelSeen: 77, KernelDropped: 4, ElapsedMs: 1234})

	got, err := summarizeRadioWindow(out)
	if err != nil {
		t.Fatal(err)
	}
	want := radioWindowCounts{Frames: 2, Stop: "frame_cap", KernelSeen: 77, KernelDropped: 4, ElapsedMs: 1234}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Every stop reason the helper can write has a word on the stack's side, in the same order.
	words := map[wire.CaptureStop]string{wire.StopWindow: "window", wire.StopFrameCap: "frame_cap", wire.StopNoInterface: "no_interface", wire.StopInterfaceDown: "interface_down", wire.StopSocketError: "socket_error", wire.StopUnsupported: "unsupported"}
	for stop, word := range words {
		raw := wire.AppendRadioTrailer(nil, wire.RadioTrailer{Stop: stop})
		if c, err := summarizeRadioWindow(raw); err != nil || c.Stop != word {
			t.Errorf("stop %d: %+v %v", stop, c, err)
		}
	}
	// A cut run (no trailer) is read the same way on both sides.
	cut, err := summarizeRadioWindow(wire.AppendRadioRecord(nil, 1, frame))
	if _, _, werr := wire.DecodeRadio(wire.AppendRadioRecord(nil, 1, frame)); err != nil || werr != nil || cut.Stop != "cut" || cut.Frames != 1 {
		t.Errorf("%+v %v %v", cut, err, werr)
	}
}

func TestTheManualListenAnswersCountsNotFrames(t *testing.T) {
	var out []byte
	out = wire.AppendRadioRecord(out, 5, make([]byte, 40))
	out = wire.AppendRadioTrailer(out, wire.RadioTrailer{Frames: 1})
	v, err := (helperParser{}).parse("radio_listen", out)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := v.(radioWindowCounts); !ok || c.Frames != 1 {
		t.Errorf("%#v", v)
	}
}
