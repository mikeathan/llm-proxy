package tools

import (
	"strings"
	"testing"
)

// The fake credentials below are assembled at runtime so the secret scanner does not read the test source as holding one.
func join(sep string, parts ...string) string { return strings.Join(parts, sep) }

func TestRedactForRecording_MasksSecretShapes(t *testing.T) {
	jwt := join(".", "eyJhbGciOiJIUzI1NiJ9", "eyJzdWIiOiIxMjM0NTY3ODkwIn0", "dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	slack := join("-", "xoxb", "1234567890", "abcdefghij")
	cases := []struct{ name, in, secret string }{
		{"openai key", "key is sk-" + strings.Repeat("a1", 20), "sk-" + strings.Repeat("a1", 20)},
		{"aws key", "id AKIAABCDEFGHIJKLMNOP end", "AKIAABCDEFGHIJKLMNOP"},
		{"google key", "k=AIza" + strings.Repeat("x", 35), "AIza" + strings.Repeat("x", 35)},
		{"github token", "use ghp_" + strings.Repeat("A1", 15) + " now", "ghp_" + strings.Repeat("A1", 15)},
		{"github fine grained", "github_pat_" + strings.Repeat("a", 30), "github_pat_" + strings.Repeat("a", 30)},
		{"slack token", slack, slack},
		{"jwt", "token " + jwt + " end", jwt},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\nabc\n-----END RSA PRIVATE KEY-----", "MIIEowIBAAKCAQEA"},
		{"authorization header", "Authorization: Bearer abcdef0123456789abcdef", "abcdef0123456789abcdef"},
		{"bearer token", "send Bearer abcdefghijklmnop1234 please", "abcdefghijklmnop1234"},
		{"password assignment", "the password is Hunter2Secret!", "Hunter2Secret!"},
		{"api key assignment", `api_key: "k3y-Value_9z"`, "k3y-Value_9z"},
		{"url password", "postgres://admin:S3cr3tPass@db.internal/app", "S3cr3tPass"},
		{"telegram bot token", "bot 123456789:ABCdefGHIjklMNOpqrSTUvwxYZ0123456789", "ABCdefGHIjklMNOpqrSTUvwxYZ0123456789"},
	}
	for _, c := range cases {
		got := RedactForRecording(c.in)
		if strings.Contains(got, c.secret) {
			t.Errorf("%s: secret survived: %q", c.name, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s: no marker: %q", c.name, got)
		}
	}
}

func TestRedactForRecording_LeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{
		"The context window is 16K tokens and max_tokens is 8192.",
		"the password is required before you can continue",
		"Set the api key is not configured yet; ask the operator.",
		"Authorization is handled by the proxy.",
		"Use read_file with offset=1363 and limit=2800 to read it.",
		"Bearer of bad news",
		"https://example.com/path?x=1 and user@example.com",
		"",
		"a short note",
	} {
		if got := RedactForRecording(s); got != s {
			t.Errorf("changed ordinary text: %q -> %q", s, got)
		}
	}
}

func TestRedactForRecording_KeepsTheLabelAndIsIdempotent(t *testing.T) {
	once := RedactForRecording("Authorization: Bearer abcdef0123456789abcdef and the password is Hunter2Secret!")
	if !strings.Contains(once, "Authorization:") || !strings.Contains(once, "password is") {
		t.Errorf("the label should stay so the recording is still readable: %q", once)
	}
	if twice := RedactForRecording(once); twice != once {
		t.Errorf("redacting twice changed the text: %q -> %q", once, twice)
	}
}

func TestRedactForRecording_FastPathAndLargeInput(t *testing.T) {
	big := strings.Repeat("plain tool output with no secrets in it\n", 50_000) // ~2 MB
	if got := RedactForRecording(big); got != big {
		t.Error("a large text with nothing to mask must come back unchanged")
	}
	withSecret := big + "key sk-" + strings.Repeat("z9", 20)
	if got := RedactForRecording(withSecret); strings.Contains(got, "sk-z9") || len(got) < len(big) {
		t.Error("a secret at the end of a large text must still be masked")
	}
}
