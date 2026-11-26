package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/core/llm"
)

// OpenCode Go answers 400 MissingSessionID unless every request carries x-opencode-session (audit 2026-11-26, B4).
// A client built for an instance whose provider names a session header sends a stable id per conversation.
func TestRuntimeClientProvider_SendsTheProvidersSessionHeader(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("x-opencode-session"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	p := &RuntimeClientProvider{newClient: func(baseURL, model string, headers http.Header, local bool) Client {
		return NewLLMClient(baseURL, model, nil, headers)
	}}
	p.ensureClient(llm.ModelInstance{URL: srv.URL, ModelID: "glm", SessionHeader: "x-opencode-session"}, "glm")

	first := []Message{{Role: SystemRole, Content: "be brief"}, {Role: UserRole, Content: "hi"}}
	next := append(append([]Message{}, first...), Message{Role: AssistantRole, Content: "hello"}, Message{Role: UserRole, Content: "more"})
	other := []Message{{Role: SystemRole, Content: "be brief"}, {Role: UserRole, Content: "another chat"}}
	for _, msgs := range [][]Message{first, next, other} {
		if _, err := p.client.Chat(context.Background(), ChatRequest{Messages: msgs}); err != nil {
			t.Fatal(err)
		}
	}
	if got[0] == "" || got[0] != got[1] {
		t.Fatalf("session ids %q, want one non-empty id for both turns of a conversation", got[:2])
	}
	if got[2] == got[0] {
		t.Fatal("a second conversation reused the first one's session id")
	}

	// A provider without a session header sends none.
	p.ensureClient(llm.ModelInstance{URL: srv.URL, ModelID: "glm"}, "glm")
	if _, err := p.client.Chat(context.Background(), ChatRequest{Messages: first}); err != nil {
		t.Fatal(err)
	}
	if got[3] != "" {
		t.Fatalf("session header %q sent to a provider that does not ask for one", got[3])
	}
}
