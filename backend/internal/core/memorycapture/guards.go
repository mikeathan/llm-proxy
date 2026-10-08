package memorycapture

import (
	"regexp"
	"strings"
	"unicode"
)

// words splits text into lowercase words, keeping apostrophes inside a word.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	})
}

func containsSensitive(sentence string) bool {
	lower := strings.ToLower(sentence)
	for _, term := range sensitiveTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

// acceptable decides whether a phrase's body is worth saving.
func acceptable(body string, secret SecretCheck) bool {
	ws := words(body)
	if len(ws) < minBodyWords || len(body) > maxBodyChars || secret(body) {
		return false
	}
	return !unresolvedStarts[ws[0]] && !(len(ws) < unresolvedMaxWords && hasBareReference(ws))
}

func hasBareReference(ws []string) bool {
	for _, w := range ws {
		switch w {
		case "it", "this", "that", "these", "those", "them":
			return true
		}
	}
	return false
}

var (
	markdownHeading = regexp.MustCompile(`^#{1,6}\s`)
	listItem        = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+`)
)

// looksLikeDocument reports a message that is pasted material (a playbook, a document, a log) rather than the user
// speaking: too long, too many lines, or carrying a heading, a table or a list. Its sentences are never scanned.
func looksLikeDocument(message string) bool {
	if len(message) > maxMessageChars {
		return true
	}
	lines, items := 0, 0
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines++
		switch {
		case markdownHeading.MatchString(line), strings.Count(line, "|") >= 2:
			return true
		case listItem.MatchString(line):
			items++
		}
	}
	return lines > maxMessageLines || items >= minListItemsToSkip
}
