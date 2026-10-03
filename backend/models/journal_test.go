package models

import (
	"strings"
	"testing"
)

func TestSanitizeJournal(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain markdown is kept", "## Worked\n- query A\n- query B", "## Worked\n- query A\n- query B"},
		{"surrounding whitespace is trimmed", "\n\n  notes  \n\n", "notes"},
		{"control characters are dropped but newlines and tabs stay", "a\x07b\x1b[31m\nc\td", "ab[31m\nc\td"},
		{"carriage returns are normalised", "one\r\ntwo\rthree", "one\ntwo\nthree"},
		{"runs of blank lines collapse", "a\n\n\n\n\nb", "a\n\nb"},
		{"empty stays empty", "  \n ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeJournal(tc.in); got != tc.want {
				t.Errorf("SanitizeJournal(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("length is capped on a character boundary", func(t *testing.T) {
		got := SanitizeJournal(strings.Repeat("é", MaxJournalChars+500))
		if n := len([]rune(got)); n != MaxJournalChars {
			t.Errorf("got %d characters, want %d", n, MaxJournalChars)
		}
	})
}
