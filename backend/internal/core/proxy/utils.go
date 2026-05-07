package proxy

import (
	"fmt"
	"strings"
)

const MaxReturnChars = 3000

// TruncateResult implements a head-and-tail truncation to preserve context window.
// It keeps the first 1200 and last 1200 characters.
func TruncateResult(content string) string {
	if len(content) <= MaxReturnChars {
		return content
	}

	head := content[:1200]
	tail := content[len(content)-1200:]
	truncatedCount := len(content) - 2400

	return fmt.Sprintf("%s\n\n... [SYSTEM TRUNCATED %d CHARACTERS] ...\n\n%s\nSYSTEM NOTE: Output too large. Use targeted tools to access specific segments.", 
		head, truncatedCount, tail)
}

// TruncateLines implements line-based head-and-tail truncation.
//  Phase 3: Structural Truncation for list_directory, grep, etc.
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
