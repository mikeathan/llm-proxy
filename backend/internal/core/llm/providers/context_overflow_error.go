// context_overflow_error.go — provider-edge classification of "the prompt does
// not fit the model's window" 400s into a typed error, the sibling of
// output_cap_error.go. llama.cpp reports both token counts as JSON fields;
// OpenAI-style, Anthropic and Gemini APIs put them in prose. Parsing is
// unavoidable but confined to this file and converted to a typed error at once:
// the agent loop sizes its recovery from the counts and never sees a string.
package providers

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrContextOverflow is the sentinel for an upstream rejection because the
// request is larger than the model's context window.
var ErrContextOverflow = errors.New("context overflow")

// ContextOverflowError is the typed overflow error. PromptTokens is what the
// server counted in the rejected request and WindowTokens the window it
// enforced; either is 0 when the response did not say.
type ContextOverflowError struct {
	PromptTokens int
	WindowTokens int
}

func (e *ContextOverflowError) Error() string {
	if e.PromptTokens == 0 || e.WindowTokens == 0 {
		return "context overflow: the request is larger than the model's context window"
	}
	return "context overflow: the request is " + strconv.Itoa(e.PromptTokens) +
		" tokens and the model's window is " + strconv.Itoa(e.WindowTokens)
}

// Unwrap satisfies errors.Is for callers matching ErrContextOverflow.
func (e *ContextOverflowError) Unwrap() error { return ErrContextOverflow }

// contextOverflowMarkers are the lowercase phrasings that identify an overflow
// when no counts can be read. A body naming only an output cap (max_tokens)
// matches none of them, so it stays ParseOutputCapError's.
var contextOverflowMarkers = []string{
	"exceed_context_size_error",
	"context_length_exceeded",
	"context length",
	"context window",
	"context size",
	"context is too long",
	"too large to fit in the context",
	"exceeds the context",
	"maximum context",
	"n_ctx",
	"prompt is too long",
	"input token count",
	"reduce the length",
	"ctx_size",
}

// IsContextOverflowBody reports whether a response body says the request is
// larger than the model's context window. It is the one marker list; the
// failure classifier uses it for its hint.
func IsContextOverflowBody(body string) bool {
	lower := strings.ToLower(body)
	for _, m := range contextOverflowMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// contextOverflowCounts reads (prompt, window) from prose: the OpenAI / OpenRouter, Anthropic and
// Gemini phrasings; each pattern says which of its two captures is which.
var contextOverflowCounts = []struct {
	re          *regexp.Regexp
	windowFirst bool
}{
	{regexp.MustCompile(`(?i)maximum context length is (\d+) tokens.{0,80}?(?:resulted in|requested(?: about)?) (\d+) tokens`), true},
	{regexp.MustCompile(`(?i)prompt is too long: (\d+) tokens > (\d+)`), false},
	{regexp.MustCompile(`(?i)input token count \((\d+)\) exceeds the maximum number of tokens allowed \((\d+)\)`), false},
}

// llamaOverflowFields are the counts llama.cpp adds to its error object.
type llamaOverflowFields struct {
	NPromptTokens int `json:"n_prompt_tokens"`
	NCtx          int `json:"n_ctx"`
}

// ParseContextOverflowError returns a typed ContextOverflowError when the body
// says the request is larger than the model's window, nil otherwise. The counts
// are filled in when the response carries them.
func ParseContextOverflowError(body string) *ContextOverflowError {
	if !IsContextOverflowBody(body) {
		return nil
	}
	var parsed struct {
		Error llamaOverflowFields `json:"error"`
		llamaOverflowFields
	}
	if json.Unmarshal([]byte(body), &parsed) == nil {
		if f := parsed.Error; f.NPromptTokens > 0 && f.NCtx > 0 {
			return &ContextOverflowError{PromptTokens: f.NPromptTokens, WindowTokens: f.NCtx}
		}
		if f := parsed.llamaOverflowFields; f.NPromptTokens > 0 && f.NCtx > 0 {
			return &ContextOverflowError{PromptTokens: f.NPromptTokens, WindowTokens: f.NCtx}
		}
	}
	for _, p := range contextOverflowCounts {
		m := p.re.FindStringSubmatch(body)
		if len(m) != 3 {
			continue
		}
		first, _ := strconv.Atoi(m[1])
		second, _ := strconv.Atoi(m[2])
		if p.windowFirst {
			return &ContextOverflowError{PromptTokens: second, WindowTokens: first}
		}
		return &ContextOverflowError{PromptTokens: first, WindowTokens: second}
	}
	return &ContextOverflowError{}
}
