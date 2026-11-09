package catalog

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// capture_window (plan network-capture-window-plan, Constitution I.11): a short, headers-only look at the broadcast and multicast frames
// this computer receives on one interface. The program is the helper's own binary (capture.Command), so no byte of a caller's text becomes
// an argument except an interface name that matches a strict pattern and a number of seconds in a fixed range. The frame cap is a constant here.
const (
	captureTimeout = (wire.CaptureMaxSeconds + 10) * time.Second // the window plus time to write the trailer before the helper stops the program
	// captureMaxPerHour is low on purpose: a window is for a person who suspects something, not for a schedule.
	captureMaxPerHour = 12
	// captureProgram is the catalogue name of the helper's own binary; the helper binary maps it to its own path.
	captureProgram = "capture"
)

// CaptureWindowParams are the parameters of capture_window. Seconds 0 means the longest window.
type CaptureWindowParams struct {
	Iface   string `json:"iface"`
	Seconds int    `json:"seconds"`
}

// Host is "": a capture touches no host, so there is no target for a scan range to check.
func (CaptureWindowParams) Host() string { return "" }

var captureIface = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$`)

func captureWindow() Action {
	return row(Action{
		ID: "capture_window", Class: ClassNet, Switch: SwitchKey, Result: ResultBytes, Jail: Jail{Network: false},
		Caps: Caps{Timeout: captureTimeout, MaxOutputBytes: wire.CaptureMaxOutput, MaxPerHour: captureMaxPerHour},
	}, captureProgram, checkCaptureWindow, func(p CaptureWindowParams) []string {
		return []string{wire.CaptureCommand, "--iface", p.Iface, "--seconds", strconv.Itoa(p.Seconds)}
	})
}

// checkCaptureWindow requires an interface name of the allowed shape and a window of 1 to 60 seconds; 0 becomes 60, so two spellings of one
// request have one canonical form.
func checkCaptureWindow(p *CaptureWindowParams) error {
	if !captureIface.MatchString(p.Iface) {
		return fmt.Errorf("%w: iface is not an interface name", ErrInvalid)
	}
	if p.Seconds == 0 {
		p.Seconds = wire.CaptureMaxSeconds
	}
	if p.Seconds < 1 || p.Seconds > wire.CaptureMaxSeconds {
		return fmt.Errorf("%w: seconds must be between 1 and %d", ErrScope, wire.CaptureMaxSeconds)
	}
	return nil
}
