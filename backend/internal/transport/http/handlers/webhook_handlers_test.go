package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/commands"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func TestWebhookSessionKey_Format(t *testing.T) {
	key := webhookSessionKey("telegram", "8699725510")
	parts := strings.Split(key, "_")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d: %s", len(parts), key)
	}
	if parts[0] != "wb" || parts[1] != "telegram" || parts[2] != "8699725510" {
		t.Errorf("unexpected format: %s", key)
	}
	// ISO UTC timestamp suffix is 16 chars: 20060102T150405Z
	if len(parts[3]) != 16 {
		t.Errorf("expected 16-char timestamp suffix, got %d: %q", len(parts[3]), parts[3])
	}
}

func TestWebhookHandler_AgentTimezone(t *testing.T) {
	cases := []struct {
		name string
		fn   func() string
		want string
	}{
		{"no provider", nil, "UTC"},
		{"unset", func() string { return "" }, "UTC"},
		{"valid owner zone", func() string { return "Europe/London" }, "Europe/London"},
		{"invalid owner zone falls back", func() string { return "Mars/Base" }, "UTC"},
	}
	for _, tc := range cases {
		h := &WebhookHandler{OwnerTimezone: tc.fn}
		if got := h.agentTimezone(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAgentRunContext_CarriesTheConnectorType(t *testing.T) {
	if got := models.OriginConnector(agentRunContext("telegram")); got != "telegram" {
		t.Fatalf("origin = %q, want telegram", got)
	}
}

func TestWebhookHandler_PlainTextIsNotACommand(t *testing.T) {
	h := &WebhookHandler{Commands: commands.New(zoneEcho())}
	if h.handleCommand("tg", "telegram", "what is the weather") {
		t.Error("plain text must go to the assistant")
	}
	if (&WebhookHandler{}).handleCommand("tg", "telegram", "/remind x") {
		t.Error("a handler with no registry handles nothing")
	}
}

// fakeTurns answers a connector message with a fixed turn result and records the request it was given.
type fakeTurns struct {
	result chat.ExecuteResult
	err    error
	got    *chat.Request
	origin string
}

func (f *fakeTurns) Run(ctx context.Context, req *chat.Request, _ logging.Logger) (chat.ExecuteResult, error) {
	f.got = req
	f.origin = models.OriginConnector(ctx)
	return f.result, f.err
}

// sentTo is a connector that records what it was asked to send.
type sentTo struct{ sent []string }

func (s *sentTo) Send(_ context.Context, message string) error {
	s.sent = append(s.sent, message)
	return nil
}
func (s *sentTo) Name() string { return "recorder" }

// The answer of a turn started by a connector message goes back through that connector, and the turn is told which
// connector it came from. Before this was tested, the reply was read from a map the typed result never was, so a
// connector's agent answers were dropped.
func TestWebhookHandler_AgentAnswerGoesBackToTheConnector(t *testing.T) {
	turns := &fakeTurns{result: chat.ExecuteResult{Reply: "Go 1.27.3, released 2026-10-19."}}
	conn := &sentTo{}
	comm := tools.NewCommunicationTools()
	comm.AddConnector("tg", "telegram", conn)
	h := &WebhookHandler{Turns: turns, CommTools: comm, Logger: logging.NewNopLogger()}

	h.runAgentReply("tg", "telegram", &chat.Request{WorkspaceID: "ws", Message: "latest Go?"})

	if len(conn.sent) != 1 || conn.sent[0] != "Go 1.27.3, released 2026-10-19." {
		t.Errorf("connector was sent %q, want the turn's answer once", conn.sent)
	}
	if turns.origin != "telegram" || turns.got.Message != "latest Go?" {
		t.Errorf("the turn got origin %q and %+v", turns.origin, turns.got)
	}
}

func TestWebhookHandler_NoAnswerSendsNothing(t *testing.T) {
	for name, turns := range map[string]*fakeTurns{
		"an empty answer (a cancelled or unadmitted turn)": {},
		"a failed turn": {err: errors.New("model unavailable")},
	} {
		conn := &sentTo{}
		comm := tools.NewCommunicationTools()
		comm.AddConnector("tg", "telegram", conn)
		h := &WebhookHandler{Turns: turns, CommTools: comm, Logger: logging.NewNopLogger()}
		h.runAgentReply("tg", "telegram", &chat.Request{WorkspaceID: "ws", Message: "hi"})
		if len(conn.sent) != 0 {
			t.Errorf("%s: sent %q, want nothing", name, conn.sent)
		}
	}
}
