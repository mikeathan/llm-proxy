package tools

import "fmt"

const MaxReturnChars = 4000

// TruncateResult implements a head-and-tail truncation to preserve context window.
// It keeps the first 1500 and last 1500 characters, providing a summary of the truncation.
func TruncateResult(content string) string {
	if len(content) <= MaxReturnChars {
		return content
	}

	head := content[:1500]
	tail := content[len(content)-1500:]
	truncatedCount := len(content) - 3000

	return fmt.Sprintf("%s\n\n... [SYSTEM TRUNCATED %d CHARACTERS TO SAVE CONTEXT] ...\n\n%s\nSYSTEM NOTE: File too large. Use search/grep tools to target specific lines.", 
		head, truncatedCount, tail)
}
