package netaudit

import (
	"slices"
	"strings"
	"time"
)

// The certificate findings (the Network stack's handshakes, ADR-0038): what a secure web page showed in a handshake. Expired, about to expire and
// old protocol versions are the checks nmap's scripts already make (CheckCertExpired, CheckCertExpiring, CheckTLSWeakProtocol), with the
// same words, so one finding stands for both however it was found and it works without nmap. A device's own certificate is the new one.
// Evidence is fixed words and dates or versions; none of the certificate's own text is ever shown.
const CheckSelfSigned = "tls_self_signed"

// selfSignedVersion is the acceptance version of a self-signed finding: it holds until the certificate stops being self-signed, at which
// point the finding is gone anyway.
const selfSignedVersion = "self-signed"

func init() {
	checkTitles[CheckSelfSigned] = "A secure web page on this device uses a certificate it signed itself"
	remediations[CheckSelfSigned] = "A certificate a device made for itself is normal on a home network, but a browser cannot tell it from a forged one, so it cannot warn you about someone pretending to be the device. If the device lets you, install a certificate from an authority your browser trusts. Otherwise accept this finding and keep the page off the internet."
	guides[CheckSelfSigned] = FixGuide{
		Steps: []string{
			"Open the secure address of the device and port the finding names in a browser, on this network.",
			"Look at the certificate in the browser. If it names the device and its address, it is the device's own and this is expected.",
			"If the device can use a certificate from an authority such as Let's Encrypt, install one in the device's settings.",
			"If it cannot, accept this finding to say it is understood, and do not forward the page to the internet.",
		},
		Risk: "A device that expects its old certificate may need the new one installed in each program that talks to it.",
		Undo: "Put the old certificate back in the device's settings, or accept this finding.",
	}
}

// TLSFindings turns what the handshakes showed into findings, one per check and port. The dates in the evidence are the certificate's
// own, which the handshake read; now is only used to decide whether one is past or near.
func TLSFindings(facts []TLSFact, now time.Time) []Finding {
	var out []Finding
	for _, f := range facts {
		add := func(check, severity, evidence, version string) {
			out = append(out, Finding{Device: f.Addr.String(), Port: f.Port, Proto: "tcp", Check: check, Severity: severity, Evidence: evidence, Version: version})
		}
		day := f.NotAfter.UTC().Format(dateLayout)
		switch {
		case f.NotAfter.IsZero():
		case f.NotAfter.Before(now):
			add(CheckCertExpired, SeverityMedium, "The certificate expired on "+day, day)
		case f.NotAfter.Before(now.Add(certSoon)) && f.NotAfter.Sub(f.NotBefore) > certShortLived:
			add(CheckCertExpiring, SeverityLow, "The certificate expires on "+day, day)
		}
		if f.SelfSigned {
			add(CheckSelfSigned, SeverityLow, "The certificate was signed by the device itself, so a browser cannot check who it belongs to", selfSignedVersion)
		}
		if names := tlsVersionNames(f.OldVersions); names != "" {
			add(CheckTLSWeakProtocol, SeverityMedium, "It still accepts old protocol versions: "+names, names)
		}
	}
	return out
}

// tlsVersionWords are the protocol versions older than 1.2 the core has words for, named as a person reads them, keyed by the number the
// standard gives them (the stack sends numbers, never words).
var tlsVersionWords = map[uint16]string{0x0301: "TLS 1.0", 0x0302: "TLS 1.1"}

// tlsVersionNames lists the words for the versions given, in the order given, leaving out a number the core has no words for.
func tlsVersionNames(versions []uint16) string {
	var names []string
	for _, v := range versions {
		if name, ok := tlsVersionWords[v]; ok {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// mergeTLS adds the certificate findings to a run's findings. A handshake that completed is the freshest word on its port: it replaces a
// carried deep-check finding of old protocol versions (which may have been fixed since), and a finding of the same key that nmap's
// scripts already made is kept instead of repeated.
func mergeTLS(findings []Finding, facts []TLSFact, fresh []Finding) []Finding {
	handshook := map[string]bool{}
	for _, f := range facts {
		handshook[Finding{Device: f.Addr.String(), Port: f.Port, Proto: "tcp", Check: CheckTLSWeakProtocol}.Key()] = true
	}
	out := slices.DeleteFunc(slices.Clone(findings), func(f Finding) bool { return handshook[f.Key()] })
	have := map[string]bool{}
	for _, f := range out {
		have[f.Key()] = true
	}
	for _, f := range fresh {
		if !have[f.Key()] {
			out = append(out, f)
		}
	}
	return out
}
