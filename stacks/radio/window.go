package radio

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"time"
)

// A radio_listen window as the helper writes it: variable-length records (8 bytes of header, then the kept bytes of one frame) and one
// 32-byte trailer. The layout is the helper's (backend/internal/platform/privhelper/wire/radio.go); a stack cannot import the core, so this is
// its own strict reader, and a contract test in the core builds windows with the helper's writer and decodes them here. Anyone in range controls
// these bytes (Constitution I.11), so every length is checked before it is used.
const (
	recordHeader = 8
	trailerSize  = 32
	maxKept      = 512
	maxFrames    = 4000
)

var trailerMagic = [4]byte{'R', 'W', 'T', '1'}

// StopReason is why a window ended. The numbers are the helper's.
type StopReason uint8

// The reasons. Everything but StopWindow and StopFrameCap means no frame was read.
const (
	StopWindow StopReason = iota
	StopFrameCap
	StopNoInterface
	StopInterfaceDown
	StopSocketError
	StopUnsupported
	// StopCut is not written by the helper: the output ended without a trailer.
	StopCut
)

// String is the fixed word a summary shows.
func (s StopReason) String() string {
	switch s {
	case StopWindow:
		return "window"
	case StopFrameCap:
		return "frame_cap"
	case StopNoInterface:
		return "no_interface"
	case StopInterfaceDown:
		return "interface_down"
	case StopSocketError:
		return "socket_error"
	case StopUnsupported:
		return "unsupported"
	}
	return "cut"
}

// WindowRecord is one kept frame: when it arrived (milliseconds after the window began) and its first bytes.
type WindowRecord struct {
	OffsetMs uint32
	Frame    []byte
}

// Trailer is what the window says about itself. Present is false when the run was cut and the helper never wrote one.
type Trailer struct {
	Stop                      StopReason
	Frames                    uint32
	KernelSeen, KernelDropped uint32
	ElapsedMs                 uint32
	Present                   bool
}

// Window is a decoded output.
type Window struct {
	Records []WindowRecord
	Trailer Trailer
}

// ErrWindowFormat means the bytes are not a radio_listen output.
var ErrWindowFormat = errors.New("radio: not a radio_listen output")

// DecodeWindow reads an output. An output with no trailer (the run was cut) gives the whole records it holds and StopCut; a record cut in the
// middle is dropped. A record over the limit, more records than a window keeps, a trailer that disagrees with the records or one with an
// unknown reason is an error.
func DecodeWindow(data []byte) (Window, error) {
	body, tr, err := splitTrailer(data)
	if err != nil {
		return Window{}, err
	}
	var w Window
	w.Trailer = tr
	for off := 0; off+recordHeader <= len(body); {
		kept := int(binary.BigEndian.Uint16(body[off+4 : off+6]))
		end := off + recordHeader + kept
		switch {
		case kept > maxKept || len(w.Records) >= maxFrames:
			return Window{}, ErrWindowFormat
		case end > len(body):
			if tr.Present {
				return Window{}, ErrWindowFormat
			}
			w.Trailer = Trailer{Stop: StopCut, Frames: uint32(len(w.Records))}
			return w, nil
		}
		w.Records = append(w.Records, WindowRecord{OffsetMs: binary.BigEndian.Uint32(body[off : off+4]), Frame: append([]byte(nil), body[off+recordHeader:end]...)})
		off = end
	}
	if !tr.Present {
		w.Trailer = Trailer{Stop: StopCut, Frames: uint32(len(w.Records))}
		return w, nil
	}
	if uint32(len(w.Records)) != tr.Frames {
		return Window{}, ErrWindowFormat
	}
	return w, nil
}

func splitTrailer(data []byte) ([]byte, Trailer, error) {
	n := len(data)
	if n < trailerSize || [4]byte(data[n-trailerSize:n-trailerSize+4]) != trailerMagic {
		return data, Trailer{}, nil
	}
	t := data[n-trailerSize:]
	tr := Trailer{
		Stop: StopReason(t[4]), Frames: binary.BigEndian.Uint32(t[8:12]), KernelSeen: binary.BigEndian.Uint32(t[12:16]),
		KernelDropped: binary.BigEndian.Uint32(t[16:20]), ElapsedMs: binary.BigEndian.Uint32(t[20:24]), Present: true,
	}
	if tr.Stop > StopUnsupported {
		return nil, Trailer{}, ErrWindowFormat
	}
	return data[:n-trailerSize], tr, nil
}

// WindowAsked is what the window was asked to do, so the summary can say whether the adapter was where it was told to be.
type WindowAsked struct {
	Channel int
}

// WindowSummary is what one window amounts to, as plain data: counts and words, never an address or a name (Constitution I.16).
type WindowSummary struct {
	Channel  int `json:"channel"`
	Frames   int `json:"frames"`
	Recorded int `json:"recorded"`
	// OwnFrames is how many of the recorded frames were sent by one of the owner's devices.
	OwnFrames     int    `json:"own_frames"`
	Unread        int    `json:"unread"`
	Stop          string `json:"stop"`
	KernelSeen    uint32 `json:"kernel_seen"`
	KernelDropped uint32 `json:"kernel_dropped"`
	ElapsedMs     uint32 `json:"elapsed_ms"`
	OnChannel     int    `json:"on_channel"`
	OffChannel    int    `json:"off_channel"`
	HeardChannels []int  `json:"heard_channels,omitempty"`
	// ChannelMatched is true when frames were heard and every one carried the radiotap frequency of the channel asked for. The helper reads the
	// channel back too, but the service port does not carry that read-back yet, so the frames are the evidence.
	ChannelMatched bool      `json:"channel_matched"`
	At             time.Time `json:"at"`
}

// Ingest reads one window's bytes into the recorder: each record is parsed with the strict frame parser and handed to Record at the time it
// arrived (start plus its offset). A frame that does not parse is counted, not an error; the window's own bytes being malformed is.
func Ingest(ctx context.Context, rec *Recorder, raw []byte, asked WindowAsked, start time.Time) (WindowSummary, error) {
	w, err := DecodeWindow(raw)
	if err != nil {
		return WindowSummary{}, err
	}
	sum := WindowSummary{
		Channel: asked.Channel, Frames: len(w.Records), Stop: w.Trailer.Stop.String(), KernelSeen: w.Trailer.KernelSeen,
		KernelDropped: w.Trailer.KernelDropped, ElapsedMs: w.Trailer.ElapsedMs, At: start,
	}
	heard := map[int]bool{}
	for _, r := range w.Records {
		f, perr := Parse(r.Frame)
		if perr != nil {
			sum.Unread++
			continue
		}
		ch := channelOf(int(f.FreqMHz))
		if ch == asked.Channel {
			sum.OnChannel++
		} else {
			sum.OffChannel++
		}
		if ch != 0 {
			heard[ch] = true
		}
		owner, err := rec.RecordSeen(ctx, f, start.Add(time.Duration(r.OffsetMs)*time.Millisecond))
		if err != nil {
			return sum, err
		}
		sum.Recorded++
		if owner {
			sum.OwnFrames++
		}
	}
	for ch := range heard {
		sum.HeardChannels = append(sum.HeardChannels, ch)
	}
	slices.Sort(sum.HeardChannels)
	sum.ChannelMatched = sum.OnChannel > 0 && sum.OffChannel == 0
	return sum, nil
}
