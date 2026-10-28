// Package nmap is the nmap scan engine. The model never supplies a flag: every argument comes from the mode table
// below or from a typed value the caller has already validated, and nmap only ever sees addresses, never names.
package nmap

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit"
)

// profile is how hard a scan pushes. Quick uses timing suited to a local network and the common ports; thorough is
// gentler and wider (the 300 most common ports plus a few the checks need, see netaudit.ThoroughPorts; measured at about a minute per host that drops probes, where 1000 ports took over three and hit the per-host limit), for an audit that must not miss a quiet device.
type profile struct {
	timing      string
	hostTimeout string
	maxRate     string
	ports       []int
}

var (
	quick    = profile{timing: "-T4", hostTimeout: "60s", maxRate: "500"}
	thorough = profile{timing: "-T3", hostTimeout: "180s", maxRate: "300", ports: netaudit.ThoroughPorts}
)

const (
	// maxHostGroup is the most devices scanned at once.
	maxHostGroup = 64
	// scriptAllowance is the extra time a device gets in a checks scan: its scripts run inside the per-device limit.
	scriptAllowance = 120 * time.Second
	maxRetries      = "1"
	// versionIntensity keeps service detection to the light probes.
	versionIntensity = "2"
)

// baseFlags are on every scan: no name lookups (the targets are addresses), XML on stdout (nothing written to a
// file), no raw sockets even if the process is privileged, and the profile's timing, retry and rate limits.
func baseFlags(p profile, req netaudit.ScanRequest) []string {
	flags := []string{
		"-n", "--unprivileged", "-oX", "-",
		p.timing, "--max-retries", maxRetries, "--host-timeout", hostTimeout(p, req), "--max-rate", p.maxRate,
	}
	if n := len(req.Targets); n > 1 {
		// nmap scans hosts in groups of about five, one group after the next, so a slow device holds up every group
		// behind it. One group for all of them makes the scan as long as the slowest device, not the sum of the groups;
		// --max-rate still bounds the traffic.
		flags = append(flags, "--min-hostgroup", strconv.Itoa(min(n, maxHostGroup)))
	}
	return flags
}

// hostTimeout is the per-device limit for a request: the operator's choice when there is one, else the depth's
// standard, times the request's factor (a retry gives a slow device twice as long). It is always whole seconds.
func hostTimeout(p profile, req netaudit.ScanRequest) string {
	base := req.HostTimeout
	if base <= 0 {
		base, _ = time.ParseDuration(p.hostTimeout)
		if req.Mode == netaudit.ModeChecks {
			base += scriptAllowance
		}
	}
	if req.HostTimeoutFactor > 1 {
		base *= time.Duration(req.HostTimeoutFactor)
	}
	return strconv.Itoa(int(base.Seconds())) + "s"
}

// modeFlags are the flags each mode adds. It is the whole of what a mode can ask nmap to do.
var modeFlags = map[netaudit.Mode][]string{
	netaudit.ModeDiscover: {"-sn"},
	// No --open: with it nmap leaves a device that has nothing open out of its output, and a device missing from the
	// output is reported as "not scanned to completion". Open ports are picked out by state when the output is read.
	netaudit.ModePorts:    {"-sT"},
	netaudit.ModeServices: {"-sT", "-sV", "--version-intensity", versionIntensity},
	// Checks are services plus the reviewed script list (see scriptFlags); nothing in a request can add to it.
	netaudit.ModeChecks: {"-sT", "-sV", "--version-intensity", versionIntensity},
}

// flagsFor is the flags a mode adds to nmap's base flags; a checks scan also runs the reviewed scripts.
func flagsFor(m netaudit.Mode) ([]string, bool) {
	flags, ok := modeFlags[m]
	if ok && m == netaudit.ModeChecks {
		flags = append(slices.Clone(flags), scriptFlags()...)
	}
	return flags, ok
}

// scriptTimeout limits one script on one port, so a service that stops answering cannot hold a scan.
const scriptTimeout = "30s"

// scriptFlags is the whole of what a checks scan asks nmap to run: exactly netaudit.CheckScriptNames(), by name.
func scriptFlags() []string {
	return []string{"--script", strings.Join(netaudit.CheckScriptNames(), ","), "--script-timeout", scriptTimeout}
}

// Args builds nmap's argument list for an authorised request. Ports is empty for the default set or a list already
// checked by netaudit.ValidatePorts (it is checked again here, because it ends up on a command line).
//
// Which addresses to scan is decided before nmap is started: a prefiltered request lists only addresses that already
// answered, and a thorough one wants every address scanned whether it answers or not, so both skip nmap's own
// liveness check (-Pn). Only a quick, unfiltered request lets nmap look for live addresses itself.
func Args(req netaudit.ScanRequest) ([]string, error) {
	flags, ok := flagsFor(req.Mode)
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
	args := append(baseFlags(p, req), flags...)
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
		if p.ports != nil {
			return []string{"-p", joinPorts(p.ports)}, nil
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
