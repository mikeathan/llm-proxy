package catalog

import (
	"fmt"
	"strconv"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// capture_host (plan network-host-traffic-and-findings-ui, Constitution I.13): a short look at the traffic of this computer itself on one
// interface, kept as the first bytes of each frame the fixed host filter lets through. The program is the helper's own binary
// (wire.HostCommand), so no byte of a caller's text becomes an argument except an interface name that matches a strict pattern and a number of
// seconds in a fixed range. The frame cap, the output cap and the sampling are fixed in code and never taken from a caller.
const (
	hostTimeout = (wire.HostMaxSeconds + 10) * time.Second // the window plus time to write the trailer before the helper stops the program
	// hostMaxPerHour is low on purpose: a look is for a person who wants to see, not for a schedule.
	hostMaxPerHour = 12
	// hostProgram is the catalogue name of the helper's own binary in host mode; the helper binary maps it to its own path.
	hostProgram = "capture-host"
)

// CaptureHostParams are the parameters of capture_host. Seconds 0 means the longest window.
type CaptureHostParams struct {
	Iface   string `json:"iface"`
	Seconds int    `json:"seconds"`
}

// Host is "": a capture touches no host, so there is no target for a scan range to check.
func (CaptureHostParams) Host() string { return "" }

func captureHost() Action {
	return row(Action{
		ID: "capture_host", Class: ClassNet, Switch: SwitchKey, Result: ResultBytes, Jail: Jail{Network: false},
		Caps: Caps{Timeout: hostTimeout, MaxOutputBytes: wire.HostMaxOutput, MaxPerHour: hostMaxPerHour},
	}, hostProgram, checkCaptureHost, func(p CaptureHostParams) []string {
		return []string{wire.HostCommand, "--iface", p.Iface, "--seconds", strconv.Itoa(p.Seconds)}
	})
}

// checkCaptureHost requires an interface name of the allowed shape and a window of 1 to 60 seconds; 0 becomes 60, so two spellings of one
// request have one canonical form.
func checkCaptureHost(p *CaptureHostParams) error {
	if !captureIface.MatchString(p.Iface) {
		return fmt.Errorf("%w: iface is not an interface name", ErrInvalid)
	}
	if p.Seconds == 0 {
		p.Seconds = wire.HostMaxSeconds
	}
	if p.Seconds < 1 || p.Seconds > wire.HostMaxSeconds {
		return fmt.Errorf("%w: seconds must be between 1 and %d", ErrScope, wire.HostMaxSeconds)
	}
	return nil
}
