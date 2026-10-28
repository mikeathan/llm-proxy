package vulnmatch

import (
	"strings"
	"unicode"
)

// A version is cut into tokens: runs of digits ("9", "6") and runs of letters ("p"). So "9.6p1" is 9 . 6 p 1. Separators
// are . - _ + ~ and an empty piece between two of them makes the version unreadable.
type token struct {
	num  string // digits without leading zeros; "" for a word
	word string
}

func (t token) isNum() bool { return t.word == "" }

// preRelease words sort before the plain release ("1.0rc1" < "1.0"), in this order.
var preRelease = map[string]int{"dev": 0, "snapshot": 1, "alpha": 2, "beta": 3, "preview": 4, "pre": 4, "rc": 5}

// isPostRelease is a single letter after a number: "p1" in OpenSSH, "k" in OpenSSL 1.0.2k. It sorts after the plain
// release ("1.0.2k" > "1.0.2").
func isPostRelease(w string) bool { return len(w) == 1 }

// tokenize returns the tokens of v, or false when v is empty, has an empty piece or uses a word that is not known to
// order before or after a release.
func tokenize(v string) ([]token, bool) {
	if v == "" {
		return nil, false
	}
	var out []token
	for _, piece := range strings.FieldsFunc(v, isSeparator) {
		out = appendRuns(out, piece)
	}
	if len(out) == 0 || strings.Contains(v, "..") || strings.HasPrefix(v, ".") || strings.HasSuffix(v, ".") {
		return nil, false
	}
	for _, t := range out {
		if !t.isNum() && !isPostRelease(t.word) {
			if _, ok := preRelease[t.word]; !ok {
				return nil, false
			}
		}
	}
	return out, true
}

func isSeparator(r rune) bool { return strings.ContainsRune(".-_+~", r) }

// appendRuns splits one piece into digit runs and letter runs, lower-casing letters.
func appendRuns(out []token, piece string) []token {
	var run []rune
	flush := func() {
		if len(run) == 0 {
			return
		}
		if unicode.IsDigit(run[0]) {
			n := strings.TrimLeft(string(run), "0")
			out = append(out, token{num: n})
		} else {
			out = append(out, token{word: strings.ToLower(string(run))})
		}
		run = run[:0]
	}
	for _, r := range piece {
		if len(run) > 0 && unicode.IsDigit(r) != unicode.IsDigit(run[0]) {
			flush()
		}
		run = append(run, r)
	}
	flush()
	return out
}

// compareVersions orders two versions: -1, 0 or 1. The second result is false when they cannot be placed against each
// other; the caller must then not treat the pair as a match.
func compareVersions(a, b string) (int, bool) {
	ta, okA := tokenize(a)
	tb, okB := tokenize(b)
	if !okA || !okB {
		return 0, false
	}
	for i := 0; i < len(ta) || i < len(tb); i++ {
		c, ok := compareAt(ta, tb, i)
		if !ok {
			return 0, false
		}
		if c != 0 {
			return c, true
		}
	}
	return 0, true
}

// compareAt compares the tokens at position i; a missing token counts as zero against a number, and as "no suffix"
// against a word.
func compareAt(ta, tb []token, i int) (int, bool) {
	switch {
	case i >= len(ta):
		c, ok := compareToken(tb[i], nil)
		return -c, ok
	case i >= len(tb):
		return compareToken(ta[i], nil)
	}
	return compareToken(ta[i], &tb[i])
}

// compareToken orders x against y; y == nil means the other version has ended.
func compareToken(x token, y *token) (int, bool) {
	switch {
	case y == nil:
		return compareToEnd(x), true
	case x.isNum() && y.isNum():
		return compareNum(x.num, y.num), true
	case x.isNum() || y.isNum():
		return compareNumWord(x, *y)
	}
	return compareWord(x.word, y.word), true
}

// compareToEnd orders a token against the end of the other version: a number above zero or a letter suffix makes this
// one newer; a pre-release word makes it older.
func compareToEnd(x token) int {
	switch {
	case x.isNum():
		return compareNum(x.num, "")
	case isPostRelease(x.word):
		return 1
	}
	return -1
}

// compareNumWord orders a number against a word at the same position. A pre-release word is older than any number;
// a letter suffix against a number ("1.0.2k" vs "1.0.2.1") has no sensible order.
func compareNumWord(x, y token) (int, bool) {
	word, sign := y.word, 1
	if !x.isNum() {
		word, sign = x.word, -1
	}
	if isPostRelease(word) {
		return 0, false
	}
	return sign, true
}

func compareNum(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func compareWord(a, b string) int {
	ra, preA := preRelease[a]
	rb, preB := preRelease[b]
	switch {
	case preA && preB:
		return cmpInt(ra, rb)
	case preA:
		return -1
	case preB:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
