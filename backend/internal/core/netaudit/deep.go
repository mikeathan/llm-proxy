package netaudit

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// The deep check is a separate, interactive scan of one device (network-audit.md 13.J, Constitution II.3 "Intrusive runs").
// Its two scripts talk only to the scanned device but send more than ordinary connections: ssl-enum-ciphers makes dozens of
// TLS handshakes (58 against one port in the real test) and ssh-auth-methods starts an authentication with a random user
// name, which the device logs. They are therefore a list of their own, never mixed with CheckScriptNames(): a nightly audit
// asks for the first list only, and nothing parses a deep script's output on its behalf.
//
// As for the nightly checks, text a device wrote is never copied into a finding: each evaluator decides yes or no and writes
// its own fixed words.

// Deep check names.
const (
	CheckTLSWeakProtocol  = "tls_weak_protocol"
	CheckTLSWeakCipher    = "tls_weak_cipher"
	CheckSSHPasswordLogin = "ssh_password_login"
)

// deepTable maps a deep script id to the evaluator of its output. It is the whole of what a deep scan may run.
var deepTable = map[string]func(output string, now time.Time) []checkHit{
	"ssl-enum-ciphers": evalTLSCiphers,
	"ssh-auth-methods": evalSSHAuthMethods,
}

// IsDeepCheck reports whether a check name is one only a deep check produces.
func IsDeepCheck(check string) bool {
	return slices.Contains([]string{CheckTLSWeakProtocol, CheckTLSWeakCipher, CheckSSHPasswordLogin}, check)
}

// DeepScriptNames lists the scripts a deep scan runs, sorted.
func DeepScriptNames() []string {
	names := make([]string, 0, len(deepTable))
	for name := range deepTable {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// DeepFindingsOf turns the script results of a deep scan into findings: only the deep checks, on open ports. It adds no
// open-port or nightly-check finding, which the audit's own scan owns.
func DeepFindingsOf(hosts []Host) []Finding {
	var out []Finding
	for _, h := range hosts {
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			out = append(out, deepFindings(h.Addr.String(), p)...)
		}
	}
	return out
}

func deepFindings(device string, p Port) []Finding {
	scripts := slices.Clone(p.Scripts)
	slices.SortFunc(scripts, func(a, b Script) int { return strings.Compare(a.ID, b.ID) })
	var out []Finding
	for _, s := range scripts {
		eval, ok := deepTable[s.ID]
		if !ok {
			continue
		}
		for _, hit := range eval(s.Output, clock()) {
			out = append(out, Finding{
				Device: device, Port: p.Number, Proto: p.Proto, Service: serviceLabel(p),
				Check: hit.Check, Severity: hit.Severity, Evidence: hit.Evidence,
			})
		}
	}
	return out
}

// tlsVersionHeader is a protocol heading in ssl-enum-ciphers' output, such as "TLSv1.0:".
var tlsVersionHeader = regexp.MustCompile(`(?m)^\s*(SSLv2|SSLv3|TLSv1\.0|TLSv1\.1|TLSv1\.2|TLSv1\.3):\s*$`)

// tlsLeastStrength is the weakest grade (A to F) nmap gives any cipher the server accepts.
var tlsLeastStrength = regexp.MustCompile(`(?m)^\s*least strength:\s*([A-F])\s*$`)

// oldTLSNames are the versions nobody should still accept, named in our own words, oldest first.
var oldTLSNames = []struct{ header, name string }{
	{"SSLv2", "SSL 2.0"}, {"SSLv3", "SSL 3.0"}, {"TLSv1.0", "TLS 1.0"}, {"TLSv1.1", "TLS 1.1"},
}

func evalTLSCiphers(out string, _ time.Time) []checkHit {
	var hits []checkHit
	if names, high := oldTLSVersions(out); len(names) > 0 {
		severity := SeverityMedium
		if high {
			severity = SeverityHigh
		}
		hits = append(hits, checkHit{Check: CheckTLSWeakProtocol, Severity: severity, Evidence: "It still accepts old protocol versions: " + strings.Join(names, ", ")})
	}
	if m := tlsLeastStrength.FindStringSubmatch(out); m != nil && m[1] > "B" {
		severity := SeverityMedium
		if m[1] > "C" {
			severity = SeverityHigh
		}
		hits = append(hits, checkHit{Check: CheckTLSWeakCipher, Severity: severity, Evidence: "The weakest cipher it accepts is graded " + m[1] + " (A is strong, F is broken)"})
	}
	return hits
}

// oldTLSVersions names the old protocol versions the output lists, and whether one of them is SSL (high severity).
func oldTLSVersions(out string) (names []string, ssl bool) {
	listed := map[string]bool{}
	for _, m := range tlsVersionHeader.FindAllStringSubmatch(out, -1) {
		listed[m[1]] = true
	}
	for _, v := range oldTLSNames {
		if listed[v.header] {
			names = append(names, v.name)
			ssl = ssl || strings.HasPrefix(v.header, "SSL")
		}
	}
	return names, ssl
}

func evalSSHAuthMethods(out string, _ time.Time) []checkHit {
	for _, line := range strings.Split(out, "\n") {
		switch strings.TrimSpace(line) {
		case "password":
			return one(CheckSSHPasswordLogin, SeverityMedium, "The SSH server accepts password login")
		case "keyboard-interactive":
			return one(CheckSSHPasswordLogin, SeverityMedium, "The SSH server accepts keyboard-interactive login, which is usually a password")
		}
	}
	return nil
}
