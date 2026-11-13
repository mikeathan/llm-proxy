package capture

import (
	"errors"
	"flag"
	"io"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// This file is the program the privileged helper runs for the radio_listen action (Constitution I.16). By the time it starts, the helper has put
// one USB adapter in monitor mode on one channel; the program only reads: it opens a packet socket on the adapter, keeps the first bytes of
// each frame (wire.RadioSnapFor: 512 for a beacon, 64 for any other frame) and writes them as records. It sends nothing, parses no frame beyond
// the two bytes that tell a beacon, and stops at the window or the frame cap. Anyone in range controls these bytes, so the unprivileged service
// reads them with its own strict parser.

// RadioCommand is the argument that makes the helper binary run this program instead of serving a connection.
const RadioCommand = wire.RadioCommand

// RadioConfig bounds one window. MaxFrames is fixed by the catalogue's cap, never taken from a caller.
type RadioConfig struct {
	Seconds   int
	MaxFrames int
	Now       func() time.Time
}

// RunRadio reads frames until the window ends or the cap is reached and writes the records and the trailer to w. A read error ends the window
// as a socket error, still with a trailer, so the service can say why; only a failure to write is returned.
func RunRadio(src Source, w io.Writer, cfg RadioConfig) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	start := cfg.Now()
	deadline := start.Add(time.Duration(cfg.Seconds) * time.Second)
	var frames uint32
	stop := wire.StopWindow
	for {
		if int(frames) >= cfg.MaxFrames {
			stop = wire.StopFrameCap
			break
		}
		if !cfg.Now().Before(deadline) {
			break
		}
		frame, err := src.Next()
		if err != nil {
			stop = wire.StopSocketError
			break
		}
		if frame == nil {
			continue
		}
		offset := uint32(cfg.Now().Sub(start) / time.Millisecond)
		if _, err := w.Write(wire.AppendRadioRecord(nil, offset, frame)); err != nil {
			return err
		}
		frames++
	}
	seen, dropped := src.Stats()
	return writeRadioTrailer(w, wire.RadioTrailer{Stop: stop, Frames: frames, KernelSeen: seen, KernelDropped: dropped, ElapsedMs: uint32(cfg.Now().Sub(start) / time.Millisecond)})
}

// FailRadio writes a trailer for a window that could not start (no interface, the interface is down, no packet socket).
func FailRadio(w io.Writer, stop wire.CaptureStop) error {
	return writeRadioTrailer(w, wire.RadioTrailer{Stop: stop})
}

func writeRadioTrailer(w io.Writer, t wire.RadioTrailer) error {
	_, err := w.Write(wire.AppendRadioTrailer(nil, t))
	return err
}

// RadioMain runs the program with the arguments after RadioCommand: --iface NAME --seconds N. Every expected condition (no such interface, the
// interface is down, no packet socket) is a trailer and exit status 0, so the operator is told the reason. It returns the exit status.
func RadioMain(args []string, w io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet(RadioCommand, flag.ContinueOnError)
	fs.SetOutput(stderr)
	iface := fs.String("iface", "", "interface to listen on")
	seconds := fs.Int("seconds", 0, "window length in seconds")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if !ValidInterface(*iface) || *seconds < 1 || *seconds > wire.RadioMaxSeconds {
		return 2
	}
	src, stop, err := OpenRadio(*iface)
	if err != nil {
		if errors.Is(err, errNotOpened) {
			if FailRadio(w, stop) != nil {
				return 1
			}
			return 0
		}
		return 1
	}
	defer src.Close()
	if RunRadio(src, w, RadioConfig{Seconds: *seconds, MaxFrames: wire.RadioMaxFrames}) != nil {
		return 1
	}
	return 0
}
