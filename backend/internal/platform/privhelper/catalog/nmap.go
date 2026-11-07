package catalog

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The first rows run nmap with the arguments measured on vertex (docs/audits/2026-11-07-privileged-helper-n0.md): UDP scans and OS
// detection need --privileged and CAP_NET_RAW. The caps are first values for N2b to tune under the unit.
const (
	nmapProgram    = "nmap"
	nmapTimeout    = 90 * time.Second // the host timeout below plus start-up and teardown
	nmapMaxOutput  = 64 << 10
	nmapMaxPerHour = 30
	maxPorts       = 9
)

// nmapFixed are the arguments both actions share; none comes from a caller.
var nmapFixed = []string{"--privileged", "-n", "-Pn", "--max-retries", "1", "--host-timeout", "60s", "--max-rate", "20"}

// udpPorts are the only ports a UDP scan may name: the services the audit looks at (design section 4).
var udpPorts = []int{53, 67, 69, 123, 137, 161, 500, 1900, 5353}

// osDetectPorts are the fixed ports OS detection probes.
var osDetectPorts = []int{22, 80, 443}

// ScanUDPParams are the parameters of scan_udp.
type ScanUDPParams struct {
	Target string `json:"target"`
	Ports  []int  `json:"ports"`
}

// OSDetectParams are the parameters of os_detect.
type OSDetectParams struct {
	Target string `json:"target"`
}

// Host is the address the scan touches.
func (p ScanUDPParams) Host() string { return p.Target }

// Host is the address the scan touches.
func (p OSDetectParams) Host() string { return p.Target }

func scanUDP() Action {
	return row(Action{
		ID: "scan_udp", Class: ClassNet, Switch: SwitchKey, Result: ResultBytes, Jail: Jail{Network: true},
		Caps: Caps{Timeout: nmapTimeout, MaxOutputBytes: nmapMaxOutput, MaxPerHour: nmapMaxPerHour},
	}, nmapProgram, checkScanUDP, func(p ScanUDPParams) []string {
		return append(slices.Clone(nmapFixed), "-sU", "-p", joinPorts(p.Ports), p.Target)
	})
}

func osDetect() Action {
	return row(Action{
		ID: "os_detect", Class: ClassNet, Switch: SwitchKey, Result: ResultBytes, Jail: Jail{Network: true},
		Caps: Caps{Timeout: nmapTimeout, MaxOutputBytes: nmapMaxOutput, MaxPerHour: nmapMaxPerHour},
	}, nmapProgram, func(p *OSDetectParams) error { return checkTarget(&p.Target) }, func(p OSDetectParams) []string {
		return append(slices.Clone(nmapFixed), "-O", "-p", joinPorts(osDetectPorts), p.Target)
	})
}

// checkScanUDP requires a private target and a non-empty subset of the fixed port list, and sorts the ports and drops repeats so the
// canonical form is one.
func checkScanUDP(p *ScanUDPParams) error {
	if err := checkTarget(&p.Target); err != nil {
		return err
	}
	if len(p.Ports) == 0 || len(p.Ports) > maxPorts {
		return fmt.Errorf("%w: name between 1 and %d ports", ErrScope, maxPorts)
	}
	for _, port := range p.Ports {
		if !slices.Contains(udpPorts, port) {
			return fmt.Errorf("%w: port %d is not one a UDP scan may name", ErrScope, port)
		}
	}
	slices.Sort(p.Ports)
	p.Ports = slices.Compact(p.Ports)
	return nil
}

func joinPorts(ports []int) string {
	s := make([]string, len(ports))
	for i, p := range ports {
		s[i] = strconv.Itoa(p)
	}
	return strings.Join(s, ",")
}
