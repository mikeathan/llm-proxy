// Package nmap is the nmap scan engine. The model never supplies a flag: every argument comes from the mode table
// below or from a typed value the caller has already validated, and nmap only ever sees addresses, never names.
package nmap

import (
	"fmt"
	"net/netip"

	"llm-proxy/internal/core/netaudit"
)

// Limits applied to every scan so a scan cannot hammer a fragile device or run for ever.
const (
	maxRetries    = "1"
	hostTimeout   = "120s"
	maxRate       = "100"
	timingProfile = "-T3"
	topPorts      = "100"
	// versionIntensity keeps service detection to the light probes.
	versionIntensity = "2"
)

// baseFlags are on every scan: no name lookups (the targets are addresses), XML on stdout (nothing written to a
// file), no raw sockets even if the process is privileged, and the rate and time limits above.
func baseFlags() []string {
	return []string{
		"-n", "--unprivileged", "-oX", "-",
		timingProfile, "--max-retries", maxRetries, "--host-timeout", hostTimeout, "--max-rate", maxRate,
	}
}

// modeFlags are the flags each mode adds. It is the whole of what a mode can ask nmap to do.
var modeFlags = map[netaudit.Mode][]string{
	netaudit.ModeDiscover: {"-sn"},
	netaudit.ModePorts:    {"-sT", "-Pn", "--open"},
	netaudit.ModeServices: {"-sT", "-Pn", "--open", "-sV", "--version-intensity", versionIntensity},
}

// Args builds nmap's argument list for a mode. Ports is empty for the default set or a list already checked by
// netaudit.ValidatePorts (it is checked again here, because it ends up on a command line).
func Args(mode netaudit.Mode, targets []netip.Addr, ports string) ([]string, error) {
	flags, ok := modeFlags[mode]
	if !ok {
		return nil, fmt.Errorf("%w: %q", netaudit.ErrBadMode, mode)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("nmap: no targets")
	}
	args := append(baseFlags(), flags...)
	if mode != netaudit.ModeDiscover {
		portArgs, err := portFlags(ports)
		if err != nil {
			return nil, err
		}
		args = append(args, portArgs...)
	}
	for _, t := range targets {
		if !t.Is4() {
			return nil, fmt.Errorf("nmap: %s is not an IPv4 address", t)
		}
		args = append(args, t.String())
	}
	return args, nil
}

func portFlags(ports string) ([]string, error) {
	if ports == "" {
		return []string{"--top-ports", topPorts}, nil
	}
	checked, err := netaudit.ValidatePorts(ports)
	if err != nil {
		return nil, err
	}
	return []string{"-p", checked}, nil
}
