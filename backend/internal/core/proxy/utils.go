package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxReturnChars is the default cap on returned tool-output characters.
const MaxReturnChars = 3000

// DefaultTruncateMarker is the verbose marker used for tool-result truncation.
// It is a format string with one verb: %d = truncated character count. The
// head is prepended and the tail is appended automatically.
const DefaultTruncateMarker = "\n\n... [SYSTEM TRUNCATED %d CHARACTERS] ...\n\nSYSTEM NOTE: Output too large. Use targeted tools to access specific segments.\n"

// TruncateResult performs head-and-tail truncation, keeping the first and last
// `limit/2` characters. marker is formatted with the truncated character count
// (%d); an empty marker defaults to DefaultTruncateMarker. The tail is always
// appended after the marker so callers need not embed it.
func TruncateResult(content string, limit int, marker string) string {
	if limit <= 0 {
		limit = MaxReturnChars
	}
	if len(content) <= limit {
		return content
	}
	half := limit / 2
	head := content[:half]
	tail := content[len(content)-half:]
	truncatedCount := len(content) - limit
	if marker == "" {
		marker = DefaultTruncateMarker
	}
	if strings.Contains(marker, "%d") {
		marker = fmt.Sprintf(marker, truncatedCount)
	}
	return head + marker + tail
}

// TruncateResultDefault truncates with the default 3000-char limit and verbose marker.
func TruncateResultDefault(content string) string {
	return TruncateResult(content, MaxReturnChars, DefaultTruncateMarker)
}

// TruncateLines implements line-based head-and-tail truncation.
// Phase 3: Structural Truncation for list_directory, grep, etc.
func TruncateLines(lines []string, maxLines int) string {
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}

	half := maxLines / 2
	head := lines[:half]
	tail := lines[len(lines)-half:]

	return fmt.Sprintf("%s\n... [SYSTEM: %d lines omitted for space] ...\n%s",
		strings.Join(head, "\n"), len(lines)-maxLines, strings.Join(tail, "\n"))
}

// DecodeToolArgs decodes JSON tool-call arguments into target. Empty input
// returns nil and leaves target untouched (the empty-args case is a no-op
// rather than a parse error). For non-empty input it is json.Unmarshal.
func DecodeToolArgs(raw string, target any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}
