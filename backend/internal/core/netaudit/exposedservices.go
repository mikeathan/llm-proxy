package netaudit

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// Kinds of service the report lists as worth the operator's review when the scan found them reachable on the network.
const (
	KindRemoteAdmin = "Remote administration"
	KindFileSharing = "File sharing"
	KindDatabase    = "Database"
)

// exposedKinds maps a service name, as nmap reports it, to the kind it belongs to. It is by confirmed service name, not by port
// number: sensitiveForwards (router.go) is a list of port numbers for what must never face the internet, a different question
// answered from the router's table.
var exposedKinds = map[string]string{
	"ssh": KindRemoteAdmin, "telnet": KindRemoteAdmin, "ms-wbt-server": KindRemoteAdmin, "vnc": KindRemoteAdmin,
	"vnc-http": KindRemoteAdmin, "login": KindRemoteAdmin, "exec": KindRemoteAdmin, "shell": KindRemoteAdmin, "wsman": KindRemoteAdmin,
	"ftp": KindFileSharing, "microsoft-ds": KindFileSharing, "netbios-ssn": KindFileSharing, "nfs": KindFileSharing,
	"afpovertcp": KindFileSharing, "rsync": KindFileSharing, "tftp": KindFileSharing,
	"mysql": KindDatabase, "postgresql": KindDatabase, "ms-sql-s": KindDatabase, "oracle": KindDatabase, "mongod": KindDatabase,
	"mongodb": KindDatabase, "redis": KindDatabase, "memcache": KindDatabase, "memcached": KindDatabase, "couchdb": KindDatabase,
	"cassandra": KindDatabase,
}

// maxExposedRows bounds the list; the rest is counted.
const maxExposedRows = 40

// ExposedService is one open port whose service the scan identified as a remote-administration, file-sharing or database service.
type ExposedService struct {
	Device  string `json:"device"`
	Port    int    `json:"port"`
	Proto   string `json:"proto"`
	Service string `json:"service"`
	// Detail is the product and version the scan read, or "" (a product nmap only guessed is left out).
	Detail string `json:"detail,omitempty"`
	Kind   string `json:"kind"`
}

// ExposedServices lists the open ports that carry such a service, by address and then port. It is a pure function of the scan:
// a service counts only when the scan confirmed its name (a port labelled only with the usual name for its number, "tcpwrapped" or
// no name is not listed, because nothing was learned about it), a device the scan did not finish is skipped, and a port an accepted
// finding covers (see CoveredPorts) is left out, as everywhere else the operator's decision is kept.
func ExposedServices(hosts []Host, covered map[string]bool) []ExposedService {
	var out []ExposedService
	for _, h := range hosts {
		if h.Unscanned {
			continue
		}
		for _, p := range h.Ports {
			kind, ok := exposedKinds[strings.ToLower(p.Service)]
			if p.State != "open" || p.Guessed || !ok || covered[portKeyOf(h, p)] {
				continue
			}
			out = append(out, ExposedService{Device: h.Addr.String(), Port: p.Number, Proto: p.Proto, Service: p.Service, Detail: ConfirmedText(joinVersion(p)), Kind: kind})
		}
	}
	slices.SortStableFunc(out, func(a, b ExposedService) int {
		if c := addrOf(a.Device).Compare(addrOf(b.Device)); c != 0 {
			return c
		}
		return a.Port - b.Port
	})
	return out
}

func addrOf(s string) netip.Addr {
	a, _ := netip.ParseAddr(s)
	return a
}

// writeExposed adds the list as a section of the report. Every value came from the network, so each cell is escaped.
func writeExposed(b *strings.Builder, list []ExposedService) {
	if len(list) == 0 {
		return
	}
	b.WriteString("\n### Exposed services worth reviewing\n\n")
	b.WriteString("Remote-administration, file-sharing and database services the scan identified on the network. Only a service the scan confirmed is listed, and what you accepted is left out.\n\n")
	b.WriteString("| Device | Port | Service | Version | Kind |\n|---|---|---|---|---|\n")
	for _, e := range list[:min(len(list), maxExposedRows)] {
		fmt.Fprintf(b, "| %s | %d/%s | %s | %s | %s |\n", cell(e.Device), e.Port, cell(e.Proto), cell(e.Service), cell(e.Detail), e.Kind)
	}
	if len(list) > maxExposedRows {
		fmt.Fprintf(b, "\nand %d more not listed.\n", len(list)-maxExposedRows)
	}
}
