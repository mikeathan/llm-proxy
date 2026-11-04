// Package ubuntuoval reads Canonical's Ubuntu security notices in OVAL form (one file per release), keeps the part needed to
// say which installed package versions are older than the version that fixed a published flaw, and fetches that file on the
// operator's say-so through the guarded opener. It compares versions with the algorithm dpkg uses.
package ubuntuoval

import (
	"strconv"
	"strings"
)

// Compare orders two Debian package versions as dpkg does: -1 when a is older than b, 0 when they are the same version and 1
// when a is newer. A version is [epoch:]upstream[-revision]; the epoch is compared as a number, then the upstream part and the
// revision by the dpkg rule, in which a tilde sorts before everything, even the end of the string. A version that is not
// well formed is still ordered, never refused: the comparison cannot panic.
func Compare(a, b string) int {
	ae, aup, arev := split(a)
	be, bup, brev := split(b)
	switch {
	case ae != be:
		if ae < be {
			return -1
		}
		return 1
	}
	if c := compareParts(aup, bup); c != 0 {
		return c
	}
	return compareParts(arev, brev)
}

// split cuts a version into its epoch (0 when absent or not a number), upstream part and revision (after the last hyphen).
func split(v string) (epoch int, upstream, revision string) {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		if n, err := strconv.Atoi(v[:i]); err == nil && n >= 0 {
			epoch = n
		}
		v = v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// order is a character's weight in the non-digit stretches: the tilde first, then the end of the string and the digits, then the
// letters, then everything else.
func order(c byte) int {
	switch {
	case isDigit(c):
		return 0
	case isLetter(c):
		return int(c)
	case c == '~':
		return -1
	case c != 0:
		return int(c) + 256
	}
	return 0
}

func at(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// compareParts is dpkg's verrevcmp: alternating stretches of non-digits (by order) and digits (by number).
func compareParts(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if c := compareLetters(a, b, &i, &j); c != 0 {
			return c
		}
		if c := compareNumbers(a, b, &i, &j); c != 0 {
			return c
		}
	}
	return 0
}

// compareLetters compares the stretch of non-digits at the two positions, advancing both past it.
func compareLetters(a, b string, i, j *int) int {
	for (*i < len(a) && !isDigit(a[*i])) || (*j < len(b) && !isDigit(b[*j])) {
		if d := order(at(a, *i)) - order(at(b, *j)); d != 0 {
			return sign(d)
		}
		*i++
		*j++
	}
	return 0
}

// compareNumbers compares the stretch of digits at the two positions as numbers (leading zeros ignored), advancing both past it.
func compareNumbers(a, b string, i, j *int) int {
	skipZeros(a, i)
	skipZeros(b, j)
	first := 0
	for *i < len(a) && isDigit(a[*i]) && *j < len(b) && isDigit(b[*j]) {
		if first == 0 {
			first = int(a[*i]) - int(b[*j])
		}
		*i++
		*j++
	}
	switch {
	case *i < len(a) && isDigit(a[*i]):
		return 1
	case *j < len(b) && isDigit(b[*j]):
		return -1
	}
	return sign(first)
}

func skipZeros(s string, i *int) {
	for *i < len(s) && s[*i] == '0' {
		*i++
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
