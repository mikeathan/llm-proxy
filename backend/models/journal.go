// journal.go — the per-automation learning journal: short notes an unattended
// run keeps for its future self (queries that surfaced new items, sources worth
// checking, topics already saturated).
package models

import (
	"strings"
	"unicode"
)

// MaxJournalChars bounds a stored journal. The text is re-injected into every
// run's prompt, so it is kept small enough not to crowd the task.
const MaxJournalChars = 4000

// maxJournalBlankLines is the longest run of empty lines kept in a journal.
const maxJournalBlankLines = 1

// SanitizeJournal makes agent-written journal text safe to store and to put
// back into a prompt: control characters (other than newline and tab) are
// dropped, line endings normalised, runs of blank lines collapsed, surrounding
// whitespace trimmed and the length capped on a character boundary.
func SanitizeJournal(text string) string {
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	blank := strings.Repeat("\n", maxJournalBlankLines+2)
	collapsed := strings.Repeat("\n", maxJournalBlankLines+1)
	for strings.Contains(text, blank) {
		text = strings.ReplaceAll(text, blank, collapsed)
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > MaxJournalChars {
		text = string(r[:MaxJournalChars])
	}
	return text
}
