package assistant

import (
	"regexp"
	"strings"
)

// FilterStreamingMarkup checks if the streaming content contains common
// technical markup (like JSON blocks, tool call signatures, etc.) that
// shouldn't be flashed to the UI. If detected, it truncates the string
// at the start of the markup and flags that a tool call is occurring.
func FilterStreamingMarkup(content string) (displayContent string, hasToolCall bool) {
	cutoffPatterns := []string{
		"<function-name>", "</function-name>", "<args-json-object>", "</args-json-object>",
		"<tools>", "functions.",
		"```json", "```",
		"{\"name\":", "[{\"name\":",
		"{\"target\":", "{\"mode\":", "{\"command\":",
		"[{'type':", "{\"type\":",
	}

	displayContent = content
	for _, p := range cutoffPatterns {
		if idx := strings.Index(displayContent, p); idx != -1 {
			displayContent = displayContent[:idx]
			hasToolCall = true
			break
		}
	}
	return displayContent, hasToolCall
}

var (
	extractionRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"](.*?)['"]\s*\}?\s*\]?`),
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"]([^'"]*)`),
	}
)

// normalizeContent strips common "structured noise" (JSON/Python-style artifacts)
// that some local models leak into the text content field.
func normalizeContent(content string) string {
	content = strings.TrimSpace(content)

	// Detect and extract text from common "structured noise" blocks
	// e.g. [{'type': 'text', 'text': 'Hello'}] -> Hello
	// We handle both complete and incomplete/truncated blocks
	for _, re := range extractionRegexes {
		content = re.ReplaceAllString(content, "$1")
	}

	return strings.TrimSpace(content)
}
