package vulnindex

import (
	"net/url"
	"regexp"
	"strings"
)

// Part letters of a CPE: application, operating system, hardware.
const partChars = "aoh"

// componentPattern is what a normalised vendor, product or version may contain. The files are untrusted input: a
// component with anything else in it is dropped, not repaired.
var componentPattern = regexp.MustCompile(`^[a-z0-9._~+\-]{1,64}$`)

// versionPattern is componentPattern for versions, which may also have capitals (a build such as "1.0.2K").
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9._~+\-]{1,64}$`)

// normalise lower-cases a CPE component, decodes the percent-encoding of the 2.2 form and removes the backslash
// escapes of the 2.3 form, so the same product written either way gets the same key.
func normalise(s string) string {
	if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	return strings.ToLower(strings.ReplaceAll(s, `\`, ""))
}

// Key is the index key for a product: "part:vendor:product", each normalised. The matcher builds the same key from the
// CPE nmap printed, so both sides must call this.
func Key(part, vendor, product string) string {
	return normalise(part) + ":" + normalise(vendor) + ":" + normalise(product)
}

// validKey reports whether a key is made of one part letter and two plain components.
func validKey(part, vendor, product string) bool {
	return len(part) == 1 && strings.Contains(partChars, part) &&
		componentPattern.MatchString(normalise(vendor)) && componentPattern.MatchString(normalise(product))
}

// splitCPE23 splits a CPE 2.3 formatted string on its unescaped colons; an escaped colon stays in its component.
func splitCPE23(s string) []string {
	var parts []string
	var b strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			b.WriteRune(r)
			escaped = true
		case r == ':':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(parts, b.String())
}
