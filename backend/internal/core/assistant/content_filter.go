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
		"<function-name>", "<tools>", "functions.",
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

// normalizeContent strips common "structured noise" (JSON/Python-style artifacts)
// that some local models leak into the text content field.
func normalizeContent(content string) string {
	content = strings.TrimSpace(content)

	// Detect and extract text from common "structured noise" blocks
	// e.g. [{'type': 'text', 'text': 'Hello'}] -> Hello
	// We handle both complete and incomplete/truncated blocks
	extractionPatterns := []struct {
		pattern     string
		replacement string
	}{
		{`\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"](.*?)['"]\s*\}?\s*\]?`, "$1"},
		{`\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"]([^'"]*)`, "$1"},
	}

	for _, p := range extractionPatterns {
		re := regexp.MustCompile("(?s)" + p.pattern) // Use (?s) to allow matching across newlines in $1
		content = re.ReplaceAllString(content, p.replacement)
	}

	return strings.TrimSpace(content)
}
