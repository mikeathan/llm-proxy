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
	// ModeDeep is the interactive deep check of one device: the intrusive scripts of DeepScriptNames on the ports a scan
	// already found open. Only the operator's own request starts it (Service.DeepScan); the ordinary scan path, an agent's
	// tool call and a scheduled audit all refuse it.
	ModeDeep Mode = "deep"
)

// ModeSpec is what the platform knows about a mode.
type ModeSpec struct {
	// Description is the one-line meaning shown to people.
	Description string
	// Intrusive modes send probes beyond a connection attempt (Constitution II.3, "Intrusive runs"): the ordinary scan
	// path refuses them and they never run unattended.
	Intrusive bool
	// NeedsNmap modes cannot be done by the limited Go engine, so they never fall back to it.
	NeedsNmap bool
}

var modeTable = map[Mode]ModeSpec{
	ModeDiscover: {Description: "Find which addresses answer"},
	ModePorts:    {Description: "Find open TCP ports"},
	ModeServices: {Description: "Identify the service and version on each open port", NeedsNmap: true},
	ModeChecks:   {Description: "Identify services, then run a short fixed list of safe checks on them", NeedsNmap: true},
	ModeDeep:     {Description: "Deep check of one device: TLS versions and ciphers, SSH password login", Intrusive: true, NeedsNmap: true},
}

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	_, ok := modeTable[m]
	return ok
}

// Spec returns the mode's table row; the zero ModeSpec for an unknown mode.
func (m Mode) Spec() ModeSpec { return modeTable[m] }

// Modes lists the modes an ordinary scan accepts, sorted: every mode that is not intrusive.
func Modes() []Mode {
	out := make([]Mode, 0, len(modeTable))
	for m, spec := range modeTable {
		if !spec.Intrusive {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return out
}
