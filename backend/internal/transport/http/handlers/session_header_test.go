package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetSessionHeader(t *testing.T) {
	body := `{"model":"glm","messages":[{"role":"user","content":"hi"}]}`

	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	setSessionHeader(r, "x-opencode-session")
	if id := r.Header.Get("x-opencode-session"); !strings.HasPrefix(id, "ses_") {
		t.Fatalf("session header = %q, want a ses_ id", id)
	}
	if rest, _ := io.ReadAll(r.Body); string(rest) != body {
		t.Fatalf("body after reading = %q, want it unchanged", rest)
	}

	// A caller that sends its own session id keeps it.
	r = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("x-opencode-session", "ses_mine")
	setSessionHeader(r, "x-opencode-session")
	if got := r.Header.Get("x-opencode-session"); got != "ses_mine" {
		t.Fatalf("caller's session id replaced with %q", got)
	}

	// No header asked for: nothing added.
	r = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	setSessionHeader(r, "")
	if len(r.Header) != 0 {
		t.Fatalf("headers %v added for a provider that asks for none", r.Header)
	}
}
