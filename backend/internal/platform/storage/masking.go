package storage

import "strings"

// MaskKey redacts a secret key, preserving only a short prefix and suffix
// for identification. Example: "sk-mysecretkey12345" → "sk-...2345"
func MaskKey(key string) string {
	const (
		prefixLen = 4
		suffixLen = 4
	)
	if len(key) <= prefixLen+suffixLen {
		return "***"
	}
	return key[:prefixLen] + "..." + key[len(key)-suffixLen:]
}

// IsMasked returns true if a key string appears to be a redacted placeholder
// rather than a real credential.
func IsMasked(key string) bool {
	return strings.Contains(key, "...") ||
		strings.Contains(key, "***") ||
		strings.Contains(key, "•••") ||
		strings.Contains(key, "●●●")
}
