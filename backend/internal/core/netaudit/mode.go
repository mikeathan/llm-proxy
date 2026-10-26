package netaudit

import "slices"

// Mode is a kind of scan. The model picks one by name; the arguments each mode stands for are built in code, so
// a mode is the whole of what the model can ask a scan program to do.
type Mode string

const (
	// ModeDiscover finds which addresses answer.
	ModeDiscover Mode = "discover"
	// ModePorts finds the open TCP ports of the addresses.
	ModePorts Mode = "ports"
	// ModeServices finds the open ports and identifies the service and version behind each.
	ModeServices Mode = "services"
	// ModeChecks does everything services does, then runs a short fixed list of safe nmap scripts on what it found
	// (see CheckScriptNames): weak settings such as SMBv1, anonymous FTP or a Redis with no password.
	ModeChecks Mode = "checks"
)

// ModeSpec is what the platform knows about a mode.
type ModeSpec struct {
	// Description is the one-line meaning shown to people.
	Description string
	// Intrusive modes send probes beyond a connection attempt; they never run unattended. None exists yet.
	Intrusive bool
	// NeedsNmap modes cannot be done by the limited Go engine, so they never fall back to it.
	NeedsNmap bool
}

var modeTable = map[Mode]ModeSpec{
	ModeDiscover: {Description: "Find which addresses answer"},
	ModePorts:    {Description: "Find open TCP ports"},
	ModeServices: {Description: "Identify the service and version on each open port", NeedsNmap: true},
	ModeChecks:   {Description: "Identify services, then run a short fixed list of safe checks on them", NeedsNmap: true},
}

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	_, ok := modeTable[m]
	return ok
}

// Spec returns the mode's table row; the zero ModeSpec for an unknown mode.
func (m Mode) Spec() ModeSpec { return modeTable[m] }

// Modes lists every mode, sorted.
func Modes() []Mode {
	out := make([]Mode, 0, len(modeTable))
	for m := range modeTable {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}
