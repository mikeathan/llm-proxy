package tools

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// A tool result is capped twice: by the tool, and again by appendToolResult on the JSON-encoded string, where quotes,
// backslashes, "&", "<", ">" and newlines grow. These helpers size text by its encoded length, so a tool that stays
// inside the cap is not cut a second time in the middle.

// encodedLen is the length of s as a JSON string (quotes included), which is how a tool result is capped.
func encodedLen(s string) int {
	encoded, err := json.Marshal(s)
	if err != nil {
		return len(s)
	}
	return len(encoded)
}

// fittingPrefix returns the length in bytes of the longest prefix of s, ending on a character boundary, whose JSON
// encoding is at most budget characters; len(s) when all of s fits, 0 when not even one character does.
func fittingPrefix(s string, budget int) int {
	if encodedLen(s) <= budget {
		return len(s)
	}
	lo, hi := 0, min(len(s), budget) // an encoded byte is never shorter than the byte, so no longer prefix can fit
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if encodedLen(s[:charStart(s, mid)]) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return charStart(s, lo)
}

// charStart moves i back to the start of the character it falls in.
func charStart(s string, i int) int {
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

// fittingSuffix is fittingPrefix for the end of s: the length in bytes of the longest suffix, starting on a character
// boundary, whose JSON encoding is at most budget characters.
func fittingSuffix(s string, budget int) int {
	if encodedLen(s) <= budget {
		return len(s)
	}
	lo, hi := 0, min(len(s), budget)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if encodedLen(s[nextCharStart(s, len(s)-mid):]) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return len(s) - nextCharStart(s, len(s)-lo)
}

// nextCharStart moves i forward to the start of the next character (i itself when it is one).
func nextCharStart(s string, i int) int {
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}

// resultCapOf is how many characters of result the agent will keep for this call. A tool that cuts its own output sizes
// the cut to it, so the agent's own cut leaves the result alone; without a cap on the context it is the default
// (proxy.MaxReturnChars), and a cap below the default is raised to it.
func resultCapOf(ctx context.Context) int {
	if n, ok := models.ResultCapFrom(ctx); ok {
		return max(n, proxy.MaxReturnChars)
	}
	return proxy.MaxReturnChars
}
