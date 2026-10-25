// Package nmap is the nmap scan engine. The model never supplies a flag: every argument comes from the mode table
// below or from a typed value the caller has already validated, and nmap only ever sees addresses, never names.
package nmap

import (
	"fmt"
	"strconv"
	"strings"

	"llm-proxy/internal/core/netaudit"
)

// profile is how hard a scan pushes. Quick uses timing suited to a local network and the common ports; thorough is
// gentler and wider (the top 100 ports), for an audit that must not miss a quiet device.
type profile struct {
	timing      string
	hostTimeout string
	maxRate     string
	topPorts    string
}

var (
	quick    = profile{timing: "-T4", hostTimeout: "60s", maxRate: "500"}
	thorough = profile{timing: "-T3", hostTimeout: "120s", maxRate: "100", topPorts: "100"}
)

const (
	maxRetries = "1"
	// versionIntensity keeps service detection to the light probes.
	versionIntensity = "2"
)

// baseFlags are on every scan: no name lookups (the targets are addresses), XML on stdout (nothing written to a
// file), no raw sockets even if the process is privileged, and the profile's timing, retry and rate limits.
func baseFlags(p profile) []string {
	return []string{
		"-n", "--unprivileged", "-oX", "-",
		p.timing, "--max-retries", maxRetries, "--host-timeout", p.hostTimeout, "--max-rate", p.maxRate,
	}
}

// modeFlags are the flags each mode adds. It is the whole of what a mode can ask nmap to do.
var modeFlags = map[netaudit.Mode][]string{
	netaudit.ModeDiscover: {"-sn"},
	netaudit.ModePorts:    {"-sT", "--open"},
	netaudit.ModeServices: {"-sT", "--open", "-sV", "--version-intensity", versionIntensity},
}

// Args builds nmap's argument list for an authorised request. Ports is empty for the default set or a list already
// checked by netaudit.ValidatePorts (it is checked again here, because it ends up on a command line).
//
// Which addresses to scan is decided before nmap is started: a prefiltered request lists only addresses that already
// answered, and a thorough one wants every address scanned whether it answers or not, so both skip nmap's own
// liveness check (-Pn). Only a quick, unfiltered request lets nmap look for live addresses itself.
func Args(req netaudit.ScanRequest) ([]string, error) {
	flags, ok := modeFlags[req.Mode]
	if !ok {
		return nil, fmt.Errorf("%w: %q", netaudit.ErrBadMode, req.Mode)
	}
	if len(req.Targets) == 0 {
		return nil, fmt.Errorf("nmap: no targets")
	}
	p := quick
	if req.Thorough {
		p = thorough
	}
	args := append(baseFlags(p), flags...)
	switch {
	case req.Mode != netaudit.ModeDiscover && (req.Thorough || req.Prefiltered):
		args = append(args, "-Pn")
	default:
		args = append(args, "-PS"+joinPorts(netaudit.DiscoverPorts))
	}
	if req.Mode != netaudit.ModeDiscover {
		portArgs, err := portFlags(req.Ports, p)
		if err != nil {
			return nil, err
		}
		args = append(args, portArgs...)
	}
	for _, t := range req.Targets {
		if !t.Is4() {
			return nil, fmt.Errorf("nmap: %s is not an IPv4 address", t)
		}
		args = append(args, t.String())
	}
	return args, nil
}

func portFlags(ports string, p profile) ([]string, error) {
	if ports == "" {
		if p.topPorts != "" {
			return []string{"--top-ports", p.topPorts}, nil
		}
		return []string{"-p", joinPorts(netaudit.DefaultPorts)}, nil
	}
	checked, err := netaudit.ValidatePorts(ports)
	if err != nil {
		return nil, err
	}
	return []string{"-p", checked}, nil
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}
