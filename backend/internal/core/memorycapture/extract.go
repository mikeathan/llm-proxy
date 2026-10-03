package memorycapture

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"llm-proxy/internal/platform/memory"
)

const (
	codeFence   = "```"
	quotePrefix = ">"
	originStore = "explicit"
	leadingJunk = " \t:,;-–—"
	trailingEnd = " \t,;:"
)

var inlineCode = regexp.MustCompile("`[^`]*`")

type trigger struct {
	phrase []rune
	fam    *family
}

// Extractor finds explicit "remember …" requests in a user's message. It is a few plain steps in sequence: drop code
// and quotes, split sentences, skip filler words, match a phrase at the sentence start, apply the guards.
type Extractor struct {
	triggers []trigger
	secret   SecretCheck
}

// NewExtractor compiles the phrase table once. A nil secret check is allowed.
func NewExtractor(secret SecretCheck) *Extractor {
	if secret == nil {
		secret = noSecrets
	}
	var triggers []trigger
	for i := range families {
		for _, phrase := range families[i].triggers {
			for _, spelling := range spellings(phrase) {
				triggers = append(triggers, trigger{phrase: []rune(spelling), fam: &families[i]})
			}
		}
	}
	sort.SliceStable(triggers, func(a, b int) bool { return len(triggers[a].phrase) > len(triggers[b].phrase) })
	return &Extractor{triggers: triggers, secret: secret}
}

// Extract returns the facts the message asks to remember, in order, at most maxCapturesPerMessage.
func (e *Extractor) Extract(message string) []Candidate {
	if looksLikeDocument(message) {
		return nil
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, sentence := range sentences(stripCode(message)) {
		c, ok := e.fromSentence(sentence)
		if !ok || seen[strings.ToLower(c.Content)] {
			continue
		}
		seen[strings.ToLower(c.Content)] = true
		out = append(out, c)
		if len(out) == maxCapturesPerMessage {
			break
		}
	}
	return out
}

func (e *Extractor) fromSentence(sentence string) (Candidate, bool) {
	if containsSensitive(sentence) {
		return Candidate{}, false
	}
	orig := []rune(strings.TrimSpace(sentence))
	folded := fold(orig)
	skip := skipFillers(folded)
	t, ok := e.match(folded[skip:])
	if !ok {
		return Candidate{}, false
	}
	body := string(orig[skip+len(t.phrase):])
	content := strings.TrimRight(strings.TrimLeft(body, leadingJunk), trailingEnd)
	if content == "" || !acceptable(body, e.secret) {
		return Candidate{}, false
	}
	return Candidate{Content: content, Scope: memory.ScopeWorkspace, Mode: t.fam.mode, Origin: t.fam.name}, true
}

// match finds the longest phrase the sentence starts with, at a word boundary.
func (e *Extractor) match(folded []rune) (trigger, bool) {
	for _, t := range e.triggers {
		if hasWordPrefix(folded, t.phrase) {
			return t, true
		}
	}
	return trigger{}, false
}

// skipFillers returns how many leading runes are filler words and the punctuation after them.
func skipFillers(folded []rune) int {
	at := 0
	for advanced := true; advanced; {
		advanced = false
		at += countLeading(folded[at:], func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(leadingJunk, r) })
		for _, filler := range fillers {
			if hasWordPrefix(folded[at:], []rune(filler)) {
				at += len(filler)
				advanced = true
				break
			}
		}
	}
	return at
}

func countLeading(rs []rune, in func(rune) bool) int {
	n := 0
	for n < len(rs) && in(rs[n]) {
		n++
	}
	return n
}

// hasWordPrefix reports whether text starts with phrase and the phrase ends at a word boundary.
func hasWordPrefix(text, phrase []rune) bool {
	if len(text) < len(phrase) {
		return false
	}
	for i, r := range phrase {
		if text[i] != r {
			return false
		}
	}
	if len(text) == len(phrase) {
		return true
	}
	next := text[len(phrase)]
	return !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '\''
}

// fold lowercases and straightens apostrophes one rune for one, so offsets in the folded text are offsets in the original.
func fold(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		switch r {
		case '’', '‘', '`':
			out[i] = '\''
		default:
			out[i] = unicode.ToLower(r)
		}
	}
	return out
}

func spellings(phrase string) []string {
	out := []string{phrase}
	for from, alternatives := range contractions {
		if strings.Contains(phrase, from) {
			for _, alt := range alternatives {
				out = append(out, strings.ReplaceAll(phrase, from, alt))
			}
		}
	}
	return out
}

// stripCode removes fenced blocks, inline code and quoted lines: text the user pasted is not an instruction to us.
func stripCode(message string) string {
	var kept []string
	inFence := false
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, codeFence):
			inFence = !inFence
		case inFence, strings.HasPrefix(trimmed, quotePrefix):
		default:
			kept = append(kept, inlineCode.ReplaceAllString(line, " "))
		}
	}
	return strings.Join(kept, "\n")
}

// sentences splits on line breaks and on . ! ? followed by space or the end; questions are dropped.
func sentences(text string) []string {
	var out []string
	var cur []rune
	flush := func(question bool) {
		if s := strings.TrimSpace(string(cur)); s != "" && !question {
			out = append(out, s)
		}
		cur = cur[:0]
	}
	rs := []rune(text)
	for i, r := range rs {
		atEnd := i+1 == len(rs) || unicode.IsSpace(rs[i+1])
		switch {
		case r == '\n':
			flush(false)
		case (r == '.' || r == '!') && atEnd:
			flush(false)
		case r == '?' && atEnd:
			flush(true)
		default:
			cur = append(cur, r)
		}
	}
	flush(false)
	return out
}
