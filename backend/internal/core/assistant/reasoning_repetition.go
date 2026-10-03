package assistant

import (
	"strings"
	"unicode/utf8"
)

// Reasoning-loop detection. A thinking model can circle on one thought for
// minutes; the length ceiling alone would let it do so for as long as the model
// is allowed to think. This guard looks at what is being written instead of how
// much: it flags a stream whose recent text mostly repeats itself, whatever the
// model or provider. Measured on real thinking from four runs (24 segments up
// to 8K chars) the highest score was 0.06 against the 0.5 threshold, and loops
// of one sentence or a paragraph cycle were flagged within about 3K chars. Text
// is compared as written: mapping digits away would also flag a drafted CSV, a
// table or a per-file checklist, whose rows differ only in numbers and ids.
// Loops that only count up are left to the length ceiling.
const (
	// reasoningRepeatWindow is the length of the fragment compared with
	// earlier text. Long enough that ordinary prose rarely repeats it.
	reasoningRepeatWindow = 64
	// reasoningRepeatTail is how much recent reasoning is judged, so the cost of
	// a check is bounded however long the stream runs.
	reasoningRepeatTail = 4096
	// reasoningRepeatMinTail is the shortest tail the guard will judge; below
	// it the guard fails open.
	reasoningRepeatMinTail = 2048
	// reasoningRepeatStep is how much the reasoning must grow between checks.
	reasoningRepeatStep = 1024
	// reasoningRepeatNumerator/Denominator: the share of windows in the tail that
	// already appeared earlier in it, at or above which the stream is a loop (1/2).
	reasoningRepeatNumerator   = 1
	reasoningRepeatDenominator = 2
)

// reasoningRepeating reports whether the recent reasoning is a loop. It judges
// only once the text has grown by reasoningRepeatStep since *checkedLen, which
// it updates, so a stream is scanned a handful of times per KB rather than per
// chunk.
func reasoningRepeating(reasoning string, checkedLen *int) bool {
	if len(reasoning)-*checkedLen < reasoningRepeatStep {
		return false
	}
	*checkedLen = len(reasoning)
	tail := normalizeForRepeat(reasoningTail(reasoning))
	if len(tail) < reasoningRepeatMinTail {
		return false
	}
	return repeatCoverageAtLeast(tail, reasoningRepeatNumerator, reasoningRepeatDenominator)
}

// reasoningTail returns the last reasoningRepeatTail bytes, starting on a rune
// boundary.
func reasoningTail(s string) string {
	if len(s) <= reasoningRepeatTail {
		return s
	}
	s = s[len(s)-reasoningRepeatTail:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

// normalizeForRepeat collapses whitespace, so text that differs only in
// spacing or line breaks counts as the same text.
func normalizeForRepeat(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\n' || r == '\t' || r == '\r':
			if !space {
				b.WriteByte(' ')
			}
			space = true
		default:
			b.WriteRune(r)
			space = false
		}
	}
	return b.String()
}

// repeatCoverageAtLeast reports whether at least num/den of the text's
// reasoningRepeatWindow-byte windows already occurred earlier in the text.
func repeatCoverageAtLeast(text string, num, den int) bool {
	windows := len(text) - reasoningRepeatWindow + 1
	if windows <= 0 {
		return false
	}
	seen := make(map[string]struct{}, windows)
	repeated := 0
	for i := 0; i < windows; i++ {
		w := text[i : i+reasoningRepeatWindow]
		if _, ok := seen[w]; ok {
			repeated++
			continue
		}
		seen[w] = struct{}{}
	}
	return repeated*den >= windows*num
}
