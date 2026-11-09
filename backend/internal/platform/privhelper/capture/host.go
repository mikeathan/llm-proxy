package capture

import (
	"errors"
	"flag"
	"io"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// This file is the program the privileged helper runs for the capture_host action (Constitution I.13): it listens on one interface for a short
// window and writes, for each frame that passes the fixed host filter (filter_host.go), the first bytes the filter kept. Like the
// capture_window program it parses no frame, sends nothing, never turns on promiscuous mode and stops at the window or a limit. What it adds is
// a bound on how many frames it keeps without reading any: it counts the frames it kept in a second, and when that passes a budget it
// asks the kernel filter to keep one more in twice as many ordinary frames. A DNS message is never sampled away, so the names stay.
// The sampling rate in force is written in every record, so the service can multiply the bytes back and say they are an estimate.

// hostBudgetPerSecond is how many frames may be kept in a second before the sampling rate doubles: 800 a second is 48,000 in a 60-second
// window, which leaves room under the 60,000 record cap for the DNS messages that are never sampled.
const hostBudgetPerSecond = 800

// HostCommand is the argument that makes the helper binary run this program instead of serving a connection.
const HostCommand = wire.HostCommand

// HostSource is where the frames come from and what the sampling rate is told to.
type HostSource interface {
	Source
	// SetSample asks the kernel filter to keep one ordinary frame in n (n a power of two from 1 to the limit).
	SetSample(n uint16) error
}

// HostConfig bounds one window. The limits are fixed by the catalogue's caps, never taken from a caller.
type HostConfig struct {
	Seconds         int
	MaxFrames       int
	MaxOutput       int
	BudgetPerSecond int
	Now             func() time.Time
}

// hostWindow is the state of one window's output: the sampling rate and how many frames were kept in the second being counted.
type hostWindow struct {
	cfg     HostConfig
	src     HostSource
	w       io.Writer
	start   time.Time
	bucket  time.Time
	inSec   int
	sample  uint16
	frames  uint32
	written int
}

// RunHost reads frames until the window ends or a limit is reached and writes the records and the trailer to w. A read error, or a filter
// that cannot be changed when the budget is passed, ends the window as a socket error, still with a trailer, so the service can say why; only a
// failure to write is returned.
func RunHost(src HostSource, w io.Writer, cfg HostConfig) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	start := cfg.Now()
	win := &hostWindow{cfg: cfg, src: src, w: w, start: start, bucket: start, sample: 1}
	deadline := start.Add(time.Duration(cfg.Seconds) * time.Second)
	stop := wire.StopWindow
	for {
		if int(win.frames) >= cfg.MaxFrames {
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
		kept, werr := win.keep(frame)
		if werr != nil {
			return werr
		}
		if !kept {
			stop = wire.StopFrameCap
			break
		}
		if !win.adjust() {
			stop = wire.StopSocketError
			break
		}
	}
	seen, dropped := src.Stats()
	_, err := w.Write(wire.AppendHostTrailer(nil, wire.HostTrailer{
		Stop: stop, Frames: win.frames, KernelSeen: seen, KernelDropped: dropped, ElapsedMs: uint32(cfg.Now().Sub(start) / time.Millisecond), Sample: win.sample,
	}))
	return err
}

// keep writes one record unless it would pass the output limit; kept is false then and nothing was written.
func (h *hostWindow) keep(frame []byte) (kept bool, err error) {
	size := wire.HostRecordHeader + min(len(frame), wire.HostSnapMax)
	if h.written+size+wire.HostTrailerSize > h.cfg.MaxOutput {
		return false, nil
	}
	offset := uint32(h.cfg.Now().Sub(h.start) / time.Millisecond)
	if _, err := h.w.Write(wire.AppendHostRecord(nil, offset, h.sample, frame)); err != nil {
		return false, err
	}
	h.written += size
	h.frames++
	return true, nil
}

// adjust counts a kept frame against the second being counted, and doubles the sampling rate when the budget is passed. It reports false when
// the filter could not be changed: a window that cannot keep to its budget stops rather than run unbounded.
func (h *hostWindow) adjust() bool {
	now := h.cfg.Now()
	if now.Sub(h.bucket) >= time.Second {
		h.bucket, h.inSec = now, 0
	}
	h.inSec++
	if h.inSec <= h.cfg.BudgetPerSecond || h.sample >= hostMaxSample {
		return true
	}
	next := h.sample * 2
	if err := h.src.SetSample(next); err != nil {
		return false
	}
	h.sample, h.bucket, h.inSec = next, now, 0
	return true
}

// HostMain runs the program with the arguments after HostCommand: --iface NAME --seconds N. Every expected condition (no such interface, the
// interface is down, no packet socket) is a trailer and exit status 0, so the operator is told the reason; a status other than 0 would only
// show as "the action failed". It returns the exit status.
func HostMain(args []string, w io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet(HostCommand, flag.ContinueOnError)
	fs.SetOutput(stderr)
	iface := fs.String("iface", "", "interface to listen on")
	seconds := fs.Int("seconds", 0, "window length in seconds")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if !ValidInterface(*iface) || *seconds < 1 || *seconds > wire.HostMaxSeconds {
		return 2
	}
	src, stop, err := OpenHost(*iface)
	if err != nil {
		if errors.Is(err, errNotOpened) {
			if _, werr := w.Write(wire.AppendHostTrailer(nil, wire.HostTrailer{Stop: stop, Sample: 1})); werr != nil {
				return 1
			}
			return 0
		}
		return 1
	}
	defer src.Close()
	cfg := HostConfig{Seconds: *seconds, MaxFrames: wire.HostMaxFrames, MaxOutput: wire.HostMaxOutput, BudgetPerSecond: hostBudgetPerSecond}
	if RunHost(src, w, cfg) != nil {
		return 1
	}
	return 0
}
