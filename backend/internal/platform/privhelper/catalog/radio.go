package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// radio_probe and radio_listen (Constitution I.16, ADR-0052, ADR-0053): the radio class. Neither sends a frame. radio_probe reads what the
// system says an adapter can do; radio_listen puts one named adapter in monitor mode on one channel for a short window and returns the
// radiotap and 802.11 headers it heard. Every argument is built here from a checked interface name, a channel from a fixed list and a number
// of seconds in a fixed range. The steps that change the adapter are listed here too, with the ones that put it back, which the helper runs on
// every path.
const (
	radioProbeProgram  = "iw"
	radioListenProgram = "radio-listen"
	ipProgram          = "ip"
	// radioDefaultSeconds is the window when none is given: the plan's 20 seconds a channel.
	radioDefaultSeconds = 20
	// radioProbeTimeout covers one read of the system's wireless records.
	radioProbeTimeout   = 15 * time.Second
	radioProbeMaxOutput = 512 << 10
	// radioListenTimeout is the longest window plus the time to set the adapter up and put it back.
	radioListenTimeout = (wire.RadioMaxSeconds + 30) * time.Second
	// The hourly caps allow a cycle every 5 minutes over about six channels, and no more.
	radioProbeMaxPerHour  = 60
	radioListenMaxPerHour = 240
)

// radioChannels are the only channels a window may name: 2.4 GHz channels 1 to 13 and the 5 GHz channels of the UK and EU. 6 GHz is not built.
var radioChannels = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13,
	36, 40, 44, 48, 52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 149, 153, 157, 161, 165,
}

var radioIface = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$`)

// RadioProbeParams are the parameters of radio_probe: none.
type RadioProbeParams struct{}

// Host is "": a probe touches no host.
func (RadioProbeParams) Host() string { return "" }

// RadioListenParams are the parameters of radio_listen. Seconds 0 means the default window.
type RadioListenParams struct {
	Iface   string `json:"iface"`
	Channel int    `json:"channel"`
	Seconds int    `json:"seconds"`
}

// Host is "": a window touches no host.
func (RadioListenParams) Host() string { return "" }

func radioProbe() Action {
	return row(Action{
		ID: "radio_probe", Class: ClassRadio, Switch: SwitchKey, Result: ResultBytes, Standing: true, Jail: Jail{Network: false},
		Caps: Caps{Timeout: radioProbeTimeout, MaxOutputBytes: radioProbeMaxOutput, MaxPerHour: radioProbeMaxPerHour},
	}, radioProbeProgram, func(*RadioProbeParams) error { return nil }, func(RadioProbeParams) []string { return []string{"list"} })
}

func radioListen() Action {
	return row(Action{
		ID: "radio_listen", Class: ClassRadio, Switch: SwitchKey, Result: ResultBytes, Standing: true, Jail: Jail{Network: false},
		Caps: Caps{Timeout: radioListenTimeout, MaxOutputBytes: wire.RadioMaxOutput, MaxPerHour: radioListenMaxPerHour},
	}, radioListenProgram, checkRadioListen, func(p RadioListenParams) []string {
		return []string{wire.RadioCommand, "--iface", p.Iface, "--seconds", strconv.Itoa(p.Seconds)}
	}, func(p RadioListenParams, out *Prepared) {
		out.Iface = p.Iface
		out.Before = []Step{
			{Program: ipProgram, Args: []string{"link", "set", "dev", p.Iface, "down"}},
			{Program: radioProbeProgram, Args: []string{"dev", p.Iface, "set", "type", "monitor"}},
			{Program: ipProgram, Args: []string{"link", "set", "dev", p.Iface, "up"}},
			{Program: radioProbeProgram, Args: []string{"dev", p.Iface, "set", "channel", strconv.Itoa(p.Channel)}},
			{Program: radioProbeProgram, Args: []string{"dev", p.Iface, "info"}, Report: true},
		}
		out.After = []Step{
			{Program: ipProgram, Args: []string{"link", "set", "dev", p.Iface, "down"}},
			{Program: radioProbeProgram, Args: []string{"dev", p.Iface, "set", "type", "managed"}},
			{Program: ipProgram, Args: []string{"link", "set", "dev", p.Iface, "up"}},
		}
	})
}

// checkRadioListen requires an interface name of the allowed shape, a channel from the fixed list and a window of 1 to 30 seconds; 0
// becomes the default, so two spellings of one request have one canonical form.
func checkRadioListen(p *RadioListenParams) error {
	if !radioIface.MatchString(p.Iface) {
		return fmt.Errorf("%w: iface is not an interface name", ErrInvalid)
	}
	if !slices.Contains(radioChannels, p.Channel) {
		return fmt.Errorf("%w: channel %d is not one a window may name", ErrScope, p.Channel)
	}
	if p.Seconds == 0 {
		p.Seconds = radioDefaultSeconds
	}
	if p.Seconds < 1 || p.Seconds > wire.RadioMaxSeconds {
		return fmt.Errorf("%w: seconds must be between 1 and %d", ErrScope, wire.RadioMaxSeconds)
	}
	return nil
}
