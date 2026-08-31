// output_cap_error.go — provider-edge classification of output-cap 400s into a
// typed error.  On the rule "never parse error strings when structured data
// exists": for output-cap 400s no structured data exists — providers return
// prose.  Parsing is unavoidable but confined to THIS one infrastructure-edge
// file, converts to a typed error immediately, and the domain never sees a
// string.  It is the last resort, behind the published-cap path (§2.6 #3).
package providers

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrOutputCapExceeded is the sentinel for an upstream 400 caused by an output
// cap that exceeds what the model supports.
var ErrOutputCapExceeded = errors.New("output cap exceeded")

// OutputCapError is the typed capability error returned to the caller.  It
// carries the requested and available (recovered) token counts and wraps
// ErrOutputCapExceeded so errors.Is/errors.As work.
type OutputCapError struct {
	Requested int
	Available int
}

func (e *OutputCapError) Error() string {
	return "output cap exceeded: requested " + strconv.Itoa(e.Requested) +
		" max_tokens but the model supports at most " + strconv.Itoa(e.Available)
}

// Unwrap satisfies errors.Is for callers matching ErrOutputCapExceeded.
func (e *OutputCapError) Unwrap() error {
	return ErrOutputCapExceeded
}

// outputCapPatterns are the known provider phrasings of an output-cap 400
// (Hermes model_metadata.py:1427-1547).  Each regex captures the max allowed
// value.
//
// NOTE: "context length/window/size" phrasings are DELIBERATELY not matched
// here. A context-too-long 400 (prompt + max_tokens exceeding the serving
// window) is a different failure class — the agent loop's reactive sieve
// handles it via isContextSizeError. Matching it here misclassifies it as an
// output-cap problem (e.g. llama.cpp's "...context window is 8192 tokens"
// would surface as "requested 2730 max_tokens but the model supports at most
// 8192" — backwards), suppressing the correct recovery.
var outputCapPatterns = []*regexp.Regexp{
	// OpenAI / OpenRouter / generic: "...max_completion_tokens is greater than the maximum allowed..."
	regexp.MustCompile(`(?i)max(_completion)?_tokens[^0-9]{0,60}(?:than|exceeds|greater than)[^0-9]{0,60}(\d+)`),
	// "...the maximum allowed value is X..."
	regexp.MustCompile(`(?i)maximum allowed value[^0-9]{0,20}(\d+)`),
}

// ParseOutputCapError inspects a 400 response body and returns a typed
// OutputCapError when the body matches a known output-cap phrasing, nil
// otherwise.  Never exposed to the domain as a string.
func ParseOutputCapError(body string) *OutputCapError {
	if !strings.Contains(strings.ToLower(body), "token") && !strings.Contains(strings.ToLower(body), "max") {
		return nil
	}
	for _, re := range outputCapPatterns {
		if m := re.FindStringSubmatch(body); len(m) > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(m[len(m)-1])); err == nil && n > 0 {
				return &OutputCapError{Available: n}
			}
		}
	}
	return nil
}
