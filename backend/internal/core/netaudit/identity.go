package netaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// Checks the identity tracking produces: a service that presents a different SSH host key or TLS certificate than it did
// before. Both are read from the output of scripts the checks already run; nothing new is sent to any device.
const (
	CheckHostKeyChanged = "hostkey_changed"
	CheckCertChanged    = "cert_changed"
)

// Identity kinds.
const (
	IdentitySSH = "ssh_hostkey"
	IdentityTLS = "tls_cert"
)

const (
	// identityKeepFor is how long an identity is kept after the device was last seen; after that it starts afresh, so a
	// device that returns months later is a baseline and not a change.
	identityKeepFor = 90 * 24 * time.Hour
	maxIdentities   = 1024
	maxPrintLen     = 128
	maxPrints       = 16
)

var (
	// sshKeyPrint reads the fingerprint token of one ssh-hostkey line, with the key type that follows it.
	sshKeyPrint = regexp.MustCompile(`(?m)^\s*\d+\s+(\S+)\s+\((RSA|DSA|ECDSA|ED25519)\)`)
	certSHA256  = regexp.MustCompile(`(?m)^SHA-256:\s+([0-9a-fA-F ]+)$`)
	certSubject = regexp.MustCompile(`(?m)^Subject:\s*(.*)$`)
	certIssuer  = regexp.MustCompile(`(?m)^Issuer:\s*(.*)$`)
)

// ObservedIdentity is the identity one open port presented in a scan.
type ObservedIdentity struct {
	Device string
	Port   int
	Kind   string
	Prints []string
	Shape  string
}

// ObservedIdentities reads the fingerprints out of the checks' script output. A port whose script printed no fingerprint
// yields nothing: unknown is not "changed".
func ObservedIdentities(hosts []Host) []ObservedIdentity {
	var out []ObservedIdentity
	for _, h := range hosts {
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			for _, s := range p.Scripts {
				if o, ok := observe(h.Addr.String(), p.Number, s); ok {
					out = append(out, o)
				}
			}
		}
	}
	return out
}

func observe(device string, port int, s Script) (ObservedIdentity, bool) {
	o := ObservedIdentity{Device: device, Port: port}
	switch s.ID {
	case "ssh-hostkey":
		o.Kind = IdentitySSH
		for _, m := range sshKeyPrint.FindAllStringSubmatch(s.Output, maxPrints) {
			o.Prints = append(o.Prints, m[2]+":"+m[1])
		}
	case "ssl-cert":
		o.Kind = IdentityTLS
		if m := certSHA256.FindStringSubmatch(s.Output); m != nil {
			o.Prints = []string{"SHA-256:" + strings.Join(strings.Fields(m[1]), "")}
			o.Shape = certShape(s.Output)
		}
	default:
		return ObservedIdentity{}, false
	}
	o.Prints = cleanPrints(o.Prints)
	return o, len(o.Prints) > 0
}

// cleanPrints drops anything too long to be a fingerprint, de-duplicates and sorts.
func cleanPrints(prints []string) []string {
	prints = slices.DeleteFunc(prints, func(p string) bool { return len(p) > maxPrintLen })
	slices.Sort(prints)
	return slices.Compact(prints)
}

// certShape hashes the parts of a certificate that stay the same when it is renewed.
func certShape(out string) string {
	parts := []string{firstMatch(certSubject, out), firstMatch(certIssuer, out), firstMatch(certKeyType, out), firstMatch(certKeyBitsRe, out)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// TrackIdentities folds what a scan saw into the stored identities. It is pure. A first sighting is remembered and not
// reported. An SSH server whose new keys share one with the old set (a key added or dropped) is not changed; one whose
// keys are all new is. A certificate with a new fingerprint is changed, and is called a renewal when its subject, issuer and
// key are the same. A port the scan did not read is left alone; an identity unseen for identityKeepFor is dropped.
func TrackIdentities(stored []models.DeviceIdentity, seen []ObservedIdentity, now time.Time) []models.DeviceIdentity {
	out := make([]models.DeviceIdentity, 0, len(stored))
	for _, s := range stored {
		if now.Sub(s.Seen) <= identityKeepFor {
			out = append(out, s)
		}
	}
	for _, o := range seen {
		i := slices.IndexFunc(out, func(s models.DeviceIdentity) bool {
			return s.Device == o.Device && s.Port == o.Port && s.Kind == o.Kind
		})
		if i < 0 {
			out = append(out, models.DeviceIdentity{Device: o.Device, Port: o.Port, Kind: o.Kind, Prints: o.Prints, Shape: o.Shape, Seen: now})
			continue
		}
		out[i] = advance(out[i], o, now)
	}
	slices.SortStableFunc(out, func(a, b models.DeviceIdentity) int { return b.Seen.Compare(a.Seen) })
	if len(out) > maxIdentities {
		out = out[:maxIdentities]
	}
	return out
}

func advance(s models.DeviceIdentity, o ObservedIdentity, now time.Time) models.DeviceIdentity {
	if s.Change != nil && now.Sub(s.Change.At) > ChangeWindow {
		s.Change = nil
	}
	if replaced(s, o) {
		s.Change = &models.IdentityChange{At: now, Renewal: o.Kind == IdentityTLS && o.Shape == s.Shape}
	}
	s.Prints, s.Shape, s.Seen = o.Prints, o.Shape, now
	return s
}

// replaced reports whether the new identity is a different one: no key in common for SSH, another fingerprint for TLS.
func replaced(s models.DeviceIdentity, o ObservedIdentity) bool {
	if o.Kind == IdentityTLS {
		return !slices.Equal(s.Prints, o.Prints)
	}
	return !slices.ContainsFunc(o.Prints, func(p string) bool { return slices.Contains(s.Prints, p) })
}

// IdentityFindings turns the identities that changed lately into findings, in fixed words. They are derived from the
// stored state, so a change stays in the report until the operator accepts it or it ages out, and not only for the night it
// was noticed.
func IdentityFindings(stored []models.DeviceIdentity, now time.Time) []Finding {
	var out []Finding
	for _, s := range stored {
		if s.Change == nil || now.Sub(s.Change.At) > ChangeWindow {
			continue
		}
		f := Finding{Device: s.Device, Port: s.Port, Proto: "tcp"}
		date := s.Change.At.Format(dateLayout)
		switch {
		case s.Kind == IdentitySSH:
			f.Check, f.Severity = CheckHostKeyChanged, SeverityMedium
			f.Evidence = fmt.Sprintf("The SSH server now presents different host keys than before (changed %s). A reinstalled system does this; so does someone pretending to be the server", date)
		case s.Change.Renewal:
			f.Check, f.Severity = CheckCertChanged, SeverityLow
			f.Evidence = fmt.Sprintf("The certificate was replaced (%s) by one with the same name, issuer and key type, which is usual for a renewal", date)
		default:
			f.Check, f.Severity = CheckCertChanged, SeverityMedium
			f.Evidence = fmt.Sprintf("The certificate was replaced (%s) by one for a different name or from a different issuer, or with a different key type", date)
		}
		out = append(out, f)
	}
	return out
}

// IdentityStore keeps the identities. A missing file is none.
type IdentityStore interface {
	Read() ([]models.DeviceIdentity, error)
	Write([]models.DeviceIdentity) error
}

// Identities is the service over the stored identities. The nightly report is its only writer; the mutex keeps two
// overlapping reports from overwriting each other.
type Identities struct {
	mu    sync.Mutex
	store IdentityStore
	now   func() time.Time
}

func NewIdentities(store IdentityStore) *Identities {
	return &Identities{store: store, now: time.Now}
}

// Track records what the scan saw and returns the findings for identities that changed. A store that cannot be read is an
// error and is not overwritten.
func (v *Identities) Track(_ context.Context, hosts []Host) ([]Finding, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	stored, err := v.store.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read the stored identities: %w", err)
	}
	now := v.now()
	next := TrackIdentities(stored, ObservedIdentities(hosts), now)
	if err := v.store.Write(next); err != nil {
		return nil, err
	}
	return IdentityFindings(next, now), nil
}
