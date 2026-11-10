package netaudit

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/testssl"
)

// The deep check's closer look at TLS (plan network-host-and-tls-looks, ADR-0041). The deep check of one device already asks nmap's ssl-enum-ciphers
// about each TLS port and reports tls_weak_protocol and tls_weak_cipher (deep.go). When testssl.sh is installed it looks at the same ports too, and its
// answer replaces nmap's for the port and the check it could judge. The check ids are the same, so the keys are the same and one fact is one finding;
// the words for a protocol are the same as nmap's, and for a cipher say which weak categories were offered where nmap says a grade.
//
// It is part of the deep check and never of the nightly audit: it makes hundreds of connections, which ADR-0012 keeps for the operator's own request.
// The program is run by the core, with the address checked against the scan scope first (ADR-0039). What comes back is a typed verdict for each protocol
// and cipher category and no text from the device (testssl package), so what is written here is this program's own words.

const (
	// maxDeepTLSPorts bounds how many TLS ports one deep check looks at with the program; a look is a few hundred connections to one service.
	maxDeepTLSPorts = 3
	// DeepTLSBudget is the time the closer look may take in all, on top of DeepBudget, which the nmap part has.
	DeepTLSBudget = 6 * time.Minute
)

// tlsPorts are the TCP ports that speak TLS from the first byte; a service on any other port is looked at only when the scan named it as TLS.
var tlsPorts = map[int]bool{443: true, 8443: true, 465: true, 636: true, 853: true, 993: true, 995: true, 5061: true}

// tlsServiceNames are the scan's names for services that speak TLS from the first byte.
var tlsServiceNames = []string{"https", "ssl/", "imaps", "pop3s", "ldaps", "smtps", "ftps"}

// speaksTLS says whether an open port is one the closer look can test.
func speaksTLS(p Port) bool {
	if p.State != "open" || p.Proto != "tcp" {
		return false
	}
	name, _, _ := strings.Cut(p.Service, " ")
	for _, prefix := range tlsServiceNames {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return tlsPorts[p.Number]
}

// legacyWords and weakWords are the fixed names of what is judged, in the order a finding lists them, with the severity each carries. The protocol
// names are nmap's check's own (oldTLSNames).
var legacyWords = []struct {
	P        testssl.Protocol
	Words    string
	Severity string
}{
	{testssl.SSLv2, "SSL 2.0", SeverityHigh}, {testssl.SSLv3, "SSL 3.0", SeverityHigh},
	{testssl.TLS10, "TLS 1.0", SeverityMedium}, {testssl.TLS11, "TLS 1.1", SeverityMedium},
}

var weakWords = []struct {
	C        testssl.CipherClass
	Words    string
	Severity string
}{
	{testssl.CipherNull, "no encryption", SeverityHigh}, {testssl.CipherAnon, "no authentication", SeverityHigh},
	{testssl.CipherExport, "export-grade encryption", SeverityHigh},
	{testssl.CipherLow, "low-strength encryption", SeverityMedium}, {testssl.Cipher3DES, "3DES or IDEA", SeverityMedium},
}

// TLSJudged says which of the two checks the look could judge: a check with an unknown verdict among its categories, or none at all, is not judged,
// so the other engine's answer for it stands. The CBC suites testssl.sh calls obsoleted are not judged at all: nearly every server still offers them.
type TLSJudged struct{ Protocols, Ciphers bool }

// TLSLookFindings turns one service's typed result into the deep check's findings for it, and says which checks it judged.
func TLSLookFindings(device string, port int, res testssl.Result) ([]Finding, TLSJudged) {
	var out []Finding
	old, oldSeverity, oldJudged := oldProtocols(res)
	weak, weakSeverity, weakJudged := weakCiphers(res)
	if oldJudged && len(old) > 0 {
		out = append(out, Finding{Device: device, Port: port, Proto: "tcp", Check: CheckTLSWeakProtocol, Severity: oldSeverity,
			Evidence: "It still accepts old protocol versions: " + strings.Join(old, ", ")})
	}
	if weakJudged && len(weak) > 0 {
		out = append(out, Finding{Device: device, Port: port, Proto: "tcp", Check: CheckTLSWeakCipher, Severity: weakSeverity,
			Evidence: "It still accepts weak cipher categories: " + strings.Join(weak, ", ")})
	}
	return out, TLSJudged{Protocols: oldJudged, Ciphers: weakJudged}
}

// oldProtocols lists the old versions offered, the worst severity among them, and whether the protocols could be judged: at least one verdict and
// none unknown.
func oldProtocols(res testssl.Result) (offered []string, severity string, judged bool) {
	seen, unknown := false, false
	for _, w := range legacyWords {
		v, ok := res.Protocols[w.P]
		seen = seen || ok
		switch v {
		case testssl.Offered:
			offered = append(offered, w.Words)
			severity = worseSeverity(severity, w.Severity)
		case testssl.Unknown:
			unknown = true
		}
	}
	return offered, severity, seen && !unknown
}

// weakCiphers is oldProtocols for the weak cipher categories.
func weakCiphers(res testssl.Result) (offered []string, severity string, judged bool) {
	seen, unknown := false, false
	for _, w := range weakWords {
		v, ok := res.Ciphers[w.C]
		seen = seen || ok
		switch v {
		case testssl.Offered:
			offered = append(offered, w.Words)
			severity = worseSeverity(severity, w.Severity)
		case testssl.Unknown:
			unknown = true
		}
	}
	return offered, severity, seen && !unknown
}

// worseSeverity returns the more serious of two severities; an empty one is the least.
func worseSeverity(a, b string) string {
	rank := map[string]int{"": 0, SeverityLow: 1, SeverityMedium: 2, SeverityHigh: 3, SeverityCritical: 4}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// TLSLooker looks at one TLS service: *testssl.Engine. Available says whether the program can be used at all.
type TLSLooker interface {
	Available(ctx context.Context) error
	Look(ctx context.Context, t testssl.Target) (testssl.Result, error)
}

// WithTLSLooker adds the closer TLS look to the deep check. Without it, or with a program that is not installed, the deep check is nmap's alone.
func (s *Service) WithTLSLooker(l TLSLooker) *Service {
	s.tls = l
	return s
}

// refineTLS looks at the device's TLS ports with the program and merges what it judged into the deep check's findings: nmap's finding of a judged
// check on a looked-at port is replaced by the program's, and anything the program could not judge stays as nmap said it. A look that failed adds
// one fixed warning naming the port. A program that is not installed adds nothing: the deep check is complete without it.
func (s *Service) refineTLS(ctx context.Context, addr netip.Addr, res *Result) {
	if s.tls == nil || s.tls.Available(ctx) != nil {
		return
	}
	ports := deepTLSPorts(res.Hosts, addr)
	ctx, cancel := context.WithTimeout(ctx, DeepTLSBudget)
	defer cancel()
	for _, port := range ports {
		looked, err := s.tls.Look(ctx, testssl.Target{Addr: addr, Port: uint16(port)})
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("The closer look at the encryption on port %d did not work, so nmap's answer for it is shown.", port))
			continue
		}
		found, judged := TLSLookFindings(addr.String(), port, looked)
		res.Findings = replaceTLSFindings(res.Findings, addr.String(), port, judged, found)
	}
}

// replaceTLSFindings drops nmap's finding of each judged check for the port and adds the program's findings.
func replaceTLSFindings(have []Finding, device string, port int, judged TLSJudged, found []Finding) []Finding {
	kept := have[:0:0]
	for _, f := range have {
		same := f.Device == device && f.Port == port
		if same && ((judged.Protocols && f.Check == CheckTLSWeakProtocol) || (judged.Ciphers && f.Check == CheckTLSWeakCipher)) {
			continue
		}
		kept = append(kept, f)
	}
	return append(kept, found...)
}

// deepTLSPorts lists the TLS ports of one device in port order, cut at the cap.
func deepTLSPorts(hosts []Host, addr netip.Addr) []int {
	var ports []int
	for _, h := range hosts {
		if h.Addr != addr {
			continue
		}
		for _, p := range h.Ports {
			if speaksTLS(p) && p.Number > 0 && p.Number <= 65535 && !slices.Contains(ports, p.Number) {
				ports = append(ports, p.Number)
			}
		}
	}
	slices.Sort(ports)
	return ports[:min(len(ports), maxDeepTLSPorts)]
}
