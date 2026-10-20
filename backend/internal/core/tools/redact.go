package tools

import (
	"regexp"
	"strings"
	"unicode"
)

// A recording holds every prompt a run sent: the system prompt, hot memory, tool results, fetched pages and anything
// the operator pasted. RedactForRecording masks what looks like a credential before it is written. It is wider than
// RedactSecrets (three API-key shapes, which the guardrails also use to block tool arguments): a recording is kept on
// disk and can be read through the admin API, so it also masks tokens, headers, key blocks and URL passwords. Hermes
// (agent/redact.py) is the model: patterns start with a literal prefix and a cheap check skips the work for text that
// carries none, because a recorded request is the whole conversation and is written on every model call.

// redactionHints are the substrings (lower-case) one of which a text must contain for any pattern to match.
var redactionHints = []string{
	"sk-", "akia", "aiza", "ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "xox", "eyj", "-----begin",
	"authorization", "bearer", "password", "passwd", "passphrase", "secret", "api key", "api_key", "api-key",
	"access token", "access_token", "auth token", "auth_token", "credential", "private key", "private_key", "://", "bot",
}

var recordingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[opusr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}(?:\.[A-Za-z0-9_-]{4,})?`),
	regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----[\s\S]*?-----END[A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\b\d{8,}:[A-Za-z0-9_-]{30,}`),
}

// labelledPatterns mask only the value and keep the label, so the recording still says what was there. Group 1 is the label.
var labelledPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\bauthorization\s*:\s*(?:bearer\s+|basic\s+)?)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{16,}`),
	regexp.MustCompile(`(://[^/\s:@]+:)[^@\s/]+(@)`),
}

// credentialValue matches a credential word, its operator and the value that follows; group 1 is the label, group 2 the value.
var credentialValue = regexp.MustCompile(`(?i)(\b(?:password|passwd|passphrase|secret|api[ _-]?key|access[ _-]?token|auth[ _-]?token|credentials?|private[ _-]?key)\b['"]?\s*(?:is|are|was|=|:)\s*['"]?)([^\s'",;]+)`)

const redactedMarker = "[REDACTED]"

// RedactForRecording returns s with credential-shaped substrings replaced by "[REDACTED]". Text with nothing to mask
// is returned as it is, and masking twice gives the same text.
func RedactForRecording(s string) string {
	if !mayHoldSecret(s) {
		return s
	}
	for _, re := range SecretPatterns {
		s = re.ReplaceAllString(s, redactedMarker)
	}
	for _, re := range recordingPatterns {
		s = re.ReplaceAllString(s, redactedMarker)
	}
	for _, re := range labelledPatterns {
		s = re.ReplaceAllString(s, "${1}"+redactedMarker+"${2}")
	}
	return credentialValue.ReplaceAllStringFunc(s, maskCredentialValue)
}

// mayHoldSecret is the cheap check: false only when no hint appears, so no pattern can match.
func mayHoldSecret(s string) bool {
	if len(s) < 8 {
		return false
	}
	lower := strings.ToLower(s)
	for _, hint := range redactionHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// maskCredentialValue masks the value of "password is X" only when X looks like a credential: at least 8
// characters with a digit, a symbol or mixed case. "the password is required" is a sentence, not a secret.
func maskCredentialValue(match string) string {
	parts := credentialValue.FindStringSubmatch(match)
	if len(parts) != 3 || !looksLikeCredential(parts[2]) {
		return match
	}
	return parts[1] + redactedMarker
}

func looksLikeCredential(v string) bool {
	if v == redactedMarker || len(v) < 8 {
		return false
	}
	var upper, lower, digit, symbol bool
	for _, r := range v {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	return digit || symbol || (upper && lower)
}
