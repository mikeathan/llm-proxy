package nmap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"unicode"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/tools"
)

// Bounds on what is taken from nmap's output. A hostile device controls the text of its banner, so every string
// from the network is cut, stripped of control characters and scrubbed of secret-shaped text before it goes
// anywhere else.
const (
	maxFieldLen   = 80
	maxHosts      = 4096
	maxPortsEach  = 1024
	maxXMLBytes   = 8 << 20
	stateOpen     = "open"
	stateUp       = "up"
	protoTCP      = "tcp"
	addrTypeIPv4  = "ipv4"
	truncationEnd = "…"
)

type xmlRun struct {
	Hosts []xmlHost `xml:"host"`
}

type xmlHost struct {
	Status    xmlState     `xml:"status"`
	Addresses []xmlAddress `xml:"address"`
	Ports     []xmlPort    `xml:"ports>port"`
}

type xmlState struct {
	State string `xml:"state,attr"`
}

type xmlAddress struct {
	Addr string `xml:"addr,attr"`
	Type string `xml:"addrtype,attr"`
}

type xmlPort struct {
	Protocol string     `xml:"protocol,attr"`
	ID       int        `xml:"portid,attr"`
	State    xmlState   `xml:"state"`
	Service  xmlService `xml:"service"`
}

type xmlService struct {
	Name    string `xml:"name,attr"`
	Product string `xml:"product,attr"`
	Version string `xml:"version,attr"`
}

// ParseXML reads nmap's -oX output. Hosts without an IPv4 address are skipped; the output of an engine is data, so
// anything malformed is an error, not a guess.
func ParseXML(r io.Reader) ([]netaudit.Host, error) {
	var run xmlRun
	dec := xml.NewDecoder(io.LimitReader(r, maxXMLBytes))
	if err := dec.Decode(&run); err != nil {
		return nil, fmt.Errorf("failed to read nmap output: %w", err)
	}
	if len(run.Hosts) > maxHosts {
		run.Hosts = run.Hosts[:maxHosts]
	}
	var hosts []netaudit.Host
	for _, h := range run.Hosts {
		if host, ok := convertHost(h); ok {
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}

// ParsePartialXML reads the output of an nmap that was killed before it finished: the complete <host> elements before
// the cut are kept and the rest is dropped. It never fails; a cut that leaves no complete host gives no hosts.
func ParsePartialXML(raw []byte) []netaudit.Host {
	const endHost, endRun = "</host>", "</nmaprun>"
	cut := bytes.LastIndex(raw, []byte(endHost))
	if cut < 0 {
		return nil
	}
	hosts, err := ParseXML(bytes.NewReader(append(append([]byte{}, raw[:cut+len(endHost)]...), endRun...)))
	if err != nil {
		return nil
	}
	return hosts
}

func convertHost(h xmlHost) (netaudit.Host, bool) {
	for _, a := range h.Addresses {
		if a.Type != addrTypeIPv4 {
			continue
		}
		addr, err := netip.ParseAddr(a.Addr)
		if err != nil {
			continue
		}
		host := netaudit.Host{Addr: addr, Up: h.Status.State == stateUp}
		for i, p := range h.Ports {
			if i == maxPortsEach {
				break
			}
			if p.Protocol != protoTCP || p.ID < 1 || p.ID > 65535 {
				continue
			}
			host.Ports = append(host.Ports, netaudit.Port{
				Number: p.ID, Proto: p.Protocol, State: clean(p.State.State),
				Service: clean(p.Service.Name), Product: clean(p.Service.Product), Version: clean(p.Service.Version),
			})
		}
		return host, true
	}
	return netaudit.Host{}, false
}

// clean makes text from the network safe to carry: control and non-printing characters removed, secret-shaped text
// redacted, and the length cut.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	s = tools.RedactSecrets(strings.TrimSpace(s))
	if len([]rune(s)) > maxFieldLen {
		s = string([]rune(s)[:maxFieldLen]) + truncationEnd
	}
	return s
}
