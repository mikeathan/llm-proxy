package netaudit

import (
	"fmt"
	"net/netip"
)

// Port is one TCP port seen on a host. Service, Product and Version come from the network and are untrusted text.
type Port struct {
	Number  int    `json:"number"`
	Proto   string `json:"proto"`
	State   string `json:"state"`
	Service string `json:"service,omitempty"`
	Product string `json:"product,omitempty"`
	Version string `json:"version,omitempty"`
}

// Host is one address a scan looked at.
type Host struct {
	Addr  netip.Addr `json:"addr"`
	Up    bool       `json:"up"`
	Ports []Port     `json:"ports,omitempty"`
}

// Finding is one fact worth reporting about a host, identified by Key so a later scan can be compared with this one.
type Finding struct {
	Device   string `json:"device"`
	Port     int    `json:"port,omitempty"`
	Proto    string `json:"proto,omitempty"`
	Service  string `json:"service,omitempty"`
	Version  string `json:"version,omitempty"`
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Evidence string `json:"evidence,omitempty"`
}

// Key identifies what a finding is about, independent of its wording, for comparing scans.
func (f Finding) Key() string {
	return fmt.Sprintf("%s|%s|%d|%s", f.Device, f.Proto, f.Port, f.Check)
}

// Check names and severities of the findings a scan alone can produce.
const (
	CheckOpenPort = "open_port"

	SeverityInfo = "info"
)

// FindingsOf turns scan hosts into findings: one per open port.
func FindingsOf(hosts []Host) []Finding {
	var out []Finding
	for _, h := range hosts {
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			out = append(out, Finding{
				Device: h.Addr.String(), Port: p.Number, Proto: p.Proto, Service: p.Service, Version: joinVersion(p),
				Check: CheckOpenPort, Severity: SeverityInfo,
			})
		}
	}
	return out
}

func joinVersion(p Port) string {
	switch {
	case p.Product != "" && p.Version != "":
		return p.Product + " " + p.Version
	case p.Product != "":
		return p.Product
	}
	return p.Version
}
