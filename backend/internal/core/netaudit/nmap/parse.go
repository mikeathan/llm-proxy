package nmap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/tools"
)

// Bounds on what is taken from nmap's output. A hostile device controls the text of its banner, so every string
// from the network is cut, stripped of control characters and scrubbed of secret-shaped text before it goes
// anywhere else.
const (
	maxFieldLen  = 80
	maxHosts     = 4096
	maxPortsEach = 1024
	// maxScriptOutput bounds what is kept of one script's output; the checks only need to see a few dozen lines.
	maxScriptOutput = 4096
	maxScriptsEach  = 16
	// maxCPEsEach bounds the identifiers kept for one service; nmap prints a handful (application, system, hardware).
	maxCPEsEach = 8
	// maxCPELen bounds one identifier; a longer one is not a CPE nmap would print.
	maxCPELen     = 160
	maxXMLBytes   = 8 << 20
	stateOpen     = "open"
	stateUp       = "up"
	protoTCP      = "tcp"
	methodTable   = "table"
	addrTypeIPv4  = "ipv4"
	truncationEnd = "…"
)

type xmlRun struct {
	Hosts []xmlHost `xml:"host"`
}

type xmlHost struct {
	// TimedOut is set by nmap on a host it gave up on at --host-timeout; it prints such a host with no ports.
	TimedOut  bool         `xml:"timedout,attr"`
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
	Protocol string      `xml:"protocol,attr"`
	ID       int         `xml:"portid,attr"`
	State    xmlState    `xml:"state"`
	Service  xmlService  `xml:"service"`
	Scripts  []xmlScript `xml:"script"`
}

// xmlScript is one script's result on a port: its id and the text it printed.
type xmlScript struct {
	ID     string `xml:"id,attr"`
	Output string `xml:"output,attr"`
}

type xmlService struct {
	Name    string   `xml:"name,attr"`
	Product string   `xml:"product,attr"`
	Version string   `xml:"version,attr"`
	CPEs    []string `xml:"cpe"`
	// Method is how nmap got the name: "probed" when it talked to the service, "table" when it only looked the port up.
	Method string `xml:"method,attr"`
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
		if h.TimedOut {
			// Ran out of time: whatever it printed is incomplete, so its ports are unknown, not "none open".
			host.Unscanned = true
			return host, true
		}
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
				Guessed: p.Service.Name != "" && p.Service.Method == methodTable,
				CPEs:    convertCPEs(p.Service.CPEs),
				Scripts: convertScripts(p.Scripts),
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

// cpePattern is the CPE 2.2 URI form nmap prints: a part (application, operating system or hardware) and up to six
// components of plain characters. Anything else is not something nmap would print.
var cpePattern = regexp.MustCompile(`^cpe:/[aoh](:[A-Za-z0-9._~%+\-]*){0,6}$`)

// convertCPEs keeps the identifiers that have exactly the CPE 2.2 shape and drops the rest whole: the version inside
// one is text the device sent, so a malformed one is discarded rather than cleaned into something that looks valid.
func convertCPEs(in []string) []string {
	var out []string
	for _, c := range in {
		if len(out) == maxCPEsEach {
			break
		}
		c = strings.TrimSpace(c)
		if len(c) > maxCPELen || !cpePattern.MatchString(c) || slices.Contains(out, c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// convertScripts keeps the output of the reviewed check scripts on a port, each cleaned and bounded. Any other script
// nmap ran (it adds some of its own while identifying services) is dropped: nothing reads it, and it is text from the
// device.
func convertScripts(in []xmlScript) []netaudit.Script {
	var out []netaudit.Script
	for _, s := range in {
		if len(out) == maxScriptsEach {
			break
		}
		if !slices.Contains(netaudit.CheckScriptNames(), s.ID) {
			continue
		}
		out = append(out, netaudit.Script{ID: s.ID, Output: cleanScript(s.Output)})
	}
	return out
}

// cleanScript is clean for text that has lines: newlines are kept (the checks read it line by line), other control
// characters are removed, secret-shaped text is redacted and the length is cut.
func cleanScript(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	s = tools.RedactSecrets(s)
	if len([]rune(s)) > maxScriptOutput {
		s = string([]rune(s)[:maxScriptOutput]) + truncationEnd
	}
	return s
}
