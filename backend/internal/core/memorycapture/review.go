package memorycapture

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"llm-proxy/internal/platform/memory"
)

// Bounds on what a review reads and returns. The transcript is cut from the oldest end so the newest messages survive.
const (
	OriginReview       = "review"
	maxReviewTurns     = 12
	maxTurnChars       = 1500
	maxTranscriptChars = 6000
	maxSuggestions     = 5
	maxSuggestionChars = 2000

	roleUser      = "user"
	roleAssistant = "assistant"
)

var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// Turn is one chat message. Only user and assistant text is ever reviewed: tool results and system text may carry
// content from the web or from files, which must never become memory by way of a model's summary.
type Turn struct {
	Role string
	Text string
}

// Suggestion is a candidate the model proposed; Duplicate marks one memory already holds.
type Suggestion struct {
	Candidate
	Duplicate bool
}

// Reviewer asks a model, once and only when the user clicks, which facts in a chat are worth remembering. The model only
// proposes: the code validates every item and the user decides what is saved.
type Reviewer struct {
	completer Completer
	lib       Library
	system    string
	secret    SecretCheck
}

// NewReviewer builds a reviewer; system is the instruction text (kept with the other prompts, so it is passed in).
func NewReviewer(c Completer, lib Library, system string, secret SecretCheck) *Reviewer {
	if secret == nil {
		secret = noSecrets
	}
	return &Reviewer{completer: c, lib: lib, system: system, secret: secret}
}

// Review returns the checked suggestions for a chat. A reply that cannot be used yields no suggestions, never an error;
// only a failing model call is returned, for the caller to report.
func (r *Reviewer) Review(ctx context.Context, workspaceID string, turns []Turn) ([]Suggestion, error) {
	transcript := buildTranscript(turns)
	if transcript == "" {
		return nil, nil
	}
	reply, err := r.completer.Complete(ctx, r.system, transcript)
	if err != nil {
		return nil, fmt.Errorf("memory review: %w", err)
	}
	var out []Suggestion
	seen := map[string]bool{}
	for _, c := range parseSuggestions(reply) {
		key := strings.ToLower(c.Content)
		if seen[key] || !r.admissible(c.Content) {
			continue
		}
		seen[key] = true
		out = append(out, Suggestion{Candidate: c, Duplicate: r.lib != nil && r.lib.Has(ctx, workspaceID, c.Content)})
		if len(out) == maxSuggestions {
			break
		}
	}
	return out, nil
}

func (r *Reviewer) admissible(content string) bool {
	return len(content) <= maxSuggestionChars && !r.secret(content) && !containsSensitive(content)
}

// buildTranscript renders the newest user/assistant turns that fit the bounds, oldest first.
func buildTranscript(turns []Turn) string {
	var lines []string
	total := 0
	for i := len(turns) - 1; i >= 0 && len(lines) < maxReviewTurns; i-- {
		label, ok := roleLabel(turns[i].Role)
		text := strings.TrimSpace(turns[i].Text)
		if !ok || text == "" {
			continue
		}
		text = cutOnRune(text, maxTurnChars)
		line := label + ": " + text
		if total+len(line) > maxTranscriptChars && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
		total += len(line)
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n\n")
}

// cutOnRune shortens s to at most n bytes without splitting a multi-byte character.
func cutOnRune(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func roleLabel(role string) (string, bool) {
	switch role {
	case roleUser:
		return "User", true
	case roleAssistant:
		return "Assistant", true
	}
	return "", false
}

type rawSuggestion struct {
	Content string `json:"content"`
	Scope   string `json:"scope"`
	Mode    string `json:"mode"`
}

// parseSuggestions finds the JSON array in a model's reply whatever surrounds it (a thinking block, a code fence,
// polite prose) and returns its usable items; anything it cannot read is dropped.
func parseSuggestions(reply string) []Candidate {
	text := thinkBlock.ReplaceAllString(reply, "")
	raw, ok := firstJSONArray(text)
	if !ok {
		return nil
	}
	var out []Candidate
	for _, item := range raw {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		out = append(out, Candidate{Content: content, Scope: scopeOrDefault(item.Scope), Mode: modeOrDefault(item.Mode), Origin: OriginReview})
	}
	return out
}

// firstJSONArray decodes the first well-formed array of suggestions in text. A
// bracket in surrounding prose ("[note] ...") or after the array does not hide
// it: each "[" is tried in turn and the decoder stops at the end of the array.
func firstJSONArray(text string) ([]rawSuggestion, bool) {
	for i := 0; i < len(text); i++ {
		if text[i] != '[' {
			continue
		}
		var raw []rawSuggestion
		if err := json.NewDecoder(strings.NewReader(text[i:])).Decode(&raw); err == nil && len(raw) > 0 {
			return raw, true
		}
	}
	return nil, false
}

func scopeOrDefault(s string) memory.Scope {
	if scope := memory.Scope(s); scope.Validate() == nil {
		return scope
	}
	return memory.ScopeWorkspace
}

func modeOrDefault(s string) memory.Mode {
	if mode := memory.Mode(s); mode.Validate() == nil {
		return mode
	}
	return memory.ModeOnDemand
}
