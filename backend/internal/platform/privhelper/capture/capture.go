// Package capture is the program the privileged helper runs for the capture_window action (Constitution I.11): it listens on one interface
// for a short window, keeps at most 64 bytes of each frame that passes a fixed filter (ARP, DHCP, mDNS, neighbour discovery), and writes
// them as fixed-size records. It parses no frame, sends nothing, never turns on promiscuous mode and stops at the window or the frame cap.
// It is the helper's own binary run under the file jail with the one capability the net class holds; the unprivileged service decodes the
// records and reads the headers. The logic here is portable and tested anywhere; the Linux file only opens, filters and binds the socket.
package capture

import (
	"errors"
	"flag"
	"io"
	"regexp"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// snapLen is how many bytes of a frame are kept; the filter accepts exactly this much.
const snapLen = wire.CaptureSnap

// Command is the argument that makes the helper binary run this program instead of serving a connection.
const Command = wire.CaptureCommand

// Source is where frames come from.
type Source interface {
	// Next returns the next frame, cut to snapLen, or nil when nothing arrived within the poll interval. An error ends the window.
	Next() ([]byte, error)
	// Stats returns the socket's own counters: frames that passed the filter and frames the kernel dropped.
	Stats() (seen, dropped uint32)
	Close() error
}

// Config bounds one window. MaxFrames is fixed by the catalogue's cap, never taken from a caller.
type Config struct {
	Seconds   int
	MaxFrames int
	Now       func() time.Time
}

// Run reads frames until the window ends or the cap is reached and writes the records and the trailer to w. A read error ends the window
// as a socket error, still with a trailer, so the service can say why; only a failure to write is returned.
func Run(src Source, w io.Writer, cfg Config) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	start := cfg.Now()
	deadline := start.Add(time.Duration(cfg.Seconds) * time.Second)
	var frames uint32
	stop := wire.StopWindow
	for {
		if frames >= uint32(cfg.MaxFrames) {
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
		if _, err := w.Write(wire.AppendCaptureRecord(nil, offset, frame)); err != nil {
			return err
		}
		frames++
	}
	seen, dropped := src.Stats()
	return writeTrailer(w, wire.CaptureTrailer{Stop: stop, Frames: frames, KernelSeen: seen, KernelDropped: dropped, ElapsedMs: uint32(cfg.Now().Sub(start) / time.Millisecond)})
}

// Fail writes a trailer for a window that could not start (no interface, the interface is down, no packet socket).
func Fail(w io.Writer, stop wire.CaptureStop) error {
	return writeTrailer(w, wire.CaptureTrailer{Stop: stop})
}

func writeTrailer(w io.Writer, t wire.CaptureTrailer) error {
	_, err := w.Write(wire.AppendCaptureTrailer(nil, t))
	return err
}

// ifacePattern is a Linux interface name: at most 15 characters, none a slash or a space. The catalogue checks the same rule; the program
// checks it again because it is the one that opens the socket.
var ifacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$`)

// ValidInterface reports whether name is an interface name this program will open.
func ValidInterface(name string) bool { return ifacePattern.MatchString(name) }

// Main runs the program with the arguments after Command: --iface NAME --seconds N. Every expected condition (no such interface, the
// interface is down, no packet socket) is a trailer and exit status 0, so the operator is told the reason; a status other than 0 would only
// show as "the action failed". It returns the exit status.
func Main(args []string, w io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet(Command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	iface := fs.String("iface", "", "interface to listen on")
	seconds := fs.Int("seconds", 0, "window length in seconds")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if !ValidInterface(*iface) || *seconds < 1 || *seconds > wire.CaptureMaxSeconds {
		return 2
	}
	src, stop, err := Open(*iface)
	if err != nil {
		if errors.Is(err, errNotOpened) {
			if Fail(w, stop) != nil {
				return 1
			}
			return 0
		}
		return 1
	}
	defer src.Close()
	if Run(src, w, Config{Seconds: *seconds, MaxFrames: wire.CaptureMaxFrames}) != nil {
		return 1
	}
	return 0
}

// errNotOpened marks an expected failure to open: Open's returned stop says why.
var errNotOpened = errors.New("the capture could not start")
