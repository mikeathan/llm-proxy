// Package footprint looks at what is visible about the operator's own domains and email addresses outside the local network:
// certificates issued for a domain, when it expires, its mail-protection records, and whether an address is in known data
// breaches. Each look asks an outside service through the guarded client, only when the operator switched that service on, and
// reads from the answer only typed facts: numbers, flags and validated short names. A look that could not be made says so with a
// fixed reason; it is never read as "nothing found".
package footprint

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// Caps on what can be watched, so one operator cannot make the program send an unbounded number of requests.
const (
	MaxDomains = 10
	MaxEmails  = 10
	maxName    = 253
	maxEmail   = 254
)

var labelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// A name that is not on the public internet is refused: it would never be answered, and asking about it tells an outside service
// something about the local network.
var privateSuffixes = []string{".local", ".internal", ".lan", ".home", ".localdomain", ".arpa", ".test", ".invalid", ".example"}

// ValidateDomain checks a domain typed by the operator and returns it lower-cased: a plain public DNS name of at least two
// labels, no address, wildcard, scheme, path, port or trailing dot.
func ValidateDomain(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" || len(name) > maxName {
		return "", errors.New("enter a domain such as example.com")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return "", errors.New("a domain is a name, not an address")
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return "", errors.New("enter a domain such as example.com")
	}
	for _, l := range labels {
		if !labelRE.MatchString(l) {
			return "", fmt.Errorf("%q is not a plain domain name: use letters, digits and hyphens, with no scheme, path, port or wildcard", raw)
		}
	}
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(name, suffix) {
			return "", errors.New("that name is not on the public internet, so there is nothing to look up")
		}
	}
	return name, nil
}

var localPartRE = regexp.MustCompile(`^[a-z0-9._%+\-]{1,64}$`)

// ValidateEmail checks an address typed by the operator and returns it lower-cased: one @, a plain local part and a public domain.
func ValidateEmail(raw string) (string, error) {
	addr := strings.ToLower(strings.TrimSpace(raw))
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || len(addr) > maxEmail || !localPartRE.MatchString(local) {
		return "", errors.New("enter an address such as name@example.com")
	}
	if _, err := ValidateDomain(domain); err != nil {
		return "", errors.New("enter an address such as name@example.com")
	}
	return addr, nil
}

// Mask shows an address without giving it away: its first character, then stars, then its domain.
func Mask(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}
