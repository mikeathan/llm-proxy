package memory

import (
	"strings"
	"unicode"
)

// recallMaxTerms bounds how many words of a chat message become recall search
// terms: the terms are OR-matched, so a long pasted message must not match
// every stored fact.
const recallMaxTerms = 8

// recallMinTermLen drops one-letter fragments such as the "s" left by
// "project's" once punctuation is removed.
const recallMinTermLen = 2

// chatStopWords are words that are common in chat questions but say nothing
// about which fact is wanted. They extend isFTSStopWord, which is tuned for task
// files; a generic word left in would OR-match unrelated facts.
var chatStopWords = map[string]bool{
	"what": true, "which": true, "who": true, "whom": true, "whose": true, "when": true, "where": true, "why": true, "how": true,
	"does": true, "did": true, "done": true, "doing": true, "has": true, "have": true, "had": true, "am": true, "were": true,
	"been": true, "being": true, "would": true, "could": true, "should": true, "may": true, "might": true, "must": true, "shall": true,
	"me": true, "my": true, "mine": true, "we": true, "us": true, "our": true, "ours": true, "you": true, "your": true, "yours": true,
	"it": true, "its": true, "this": true, "that": true, "these": true, "those": true, "there": true, "here": true,
	"they": true, "them": true, "their": true, "he": true, "she": true, "his": true, "her": true,
	"project": true, "projects": true, "user": true, "please": true, "help": true, "tell": true, "know": true,
	"about": true, "any": true, "some": true, "all": true, "from": true, "into": true, "as": true, "if": true, "so": true,
	"than": true, "then": true, "also": true, "just": true, "like": true, "want": true, "need": true, "get": true,
	"give": true, "show": true, "ok": true, "okay": true, "thanks": true, "thank": true, "hi": true, "hello": true, "hey": true,
	"yes": true, "no": true,
}

// RecallQuery turns a chat message into the search terms used to recall saved
// facts for it: lower-cased letters and digits, without stop words or one-letter
// fragments, de-duplicated, at most recallMaxTerms, joined by spaces. It returns
// "" when nothing meaningful is left, and the caller then recalls nothing.
func RecallQuery(message string) string {
	words := strings.FieldsFunc(strings.ToLower(message), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool, len(words))
	terms := make([]string, 0, recallMaxTerms)
	for _, w := range words {
		if len(terms) == recallMaxTerms {
			break
		}
		if len(w) < recallMinTermLen || seen[w] || isFTSStopWord(w) || chatStopWords[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
	}
	return strings.Join(terms, " ")
}
