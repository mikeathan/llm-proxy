package handlers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/commands"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
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
	in := inbound{workspaceID: "ws", connectorName: "tg", connectorType: "telegram", chatID: "42"}
	if h.handleCommand(in, "what is the weather") {
		t.Error("plain text must go to the assistant")
	}
	if (&WebhookHandler{}).handleCommand(in, "/remind x") {
		t.Error("a handler with no registry handles nothing")
	}
}

// fakeTurns answers a connector message with a fixed turn result and records the request it was given. ran, when
// set, is closed once a turn ran (a command's turn runs in a goroutine).
type fakeTurns struct {
	result chat.ExecuteResult
	err    error
	got    *chat.Request
	origin string
	ran    chan struct{}
}

func (f *fakeTurns) Run(ctx context.Context, req *chat.Request, _ logging.Logger) (chat.ExecuteResult, error) {
	f.got = req
	f.origin = models.OriginConnector(ctx)
	if f.ran != nil {
		close(f.ran)
	}
	return f.result, f.err
}

// turnCommand is a turn command that asks the "research" agent, with helpers, and says so first.
func turnCommand() commands.Command {
	return commands.Command{
		Name: "research", Usage: "/research <question>",
		Turn: func(_ context.Context, r commands.Request) (commands.TurnStart, error) {
			if r.Args == "" {
				return commands.TurnStart{}, errors.New("ask a question after /research")
			}
			return commands.TurnStart{Spec: commands.TurnSpec{Message: r.Args, Agent: "research", UseHelpers: true}, Note: "a note"}, nil
		},
	}
}

// A turn command from a connector starts its turn the way any message does: the agent and helpers ride on the
// request, the note goes to the chat first and the answer after.
func TestWebhookHandler_TurnCommandRunsItsTurn(t *testing.T) {
	turns := &fakeTurns{result: chat.ExecuteResult{Reply: "the answer"}, ran: make(chan struct{})}
	conn := &sentTo{}
	comm := tools.NewCommunicationTools()
	comm.AddConnector("tg", "telegram", conn)
	h := &WebhookHandler{Commands: commands.New(turnCommand()), Turns: turns, CommTools: comm, Logger: logging.NewNopLogger()}

	if !h.handleCommand(inbound{workspaceID: "ws", connectorName: "tg", connectorType: "telegram", chatID: "42"}, "/research compare Go and Rust") {
		t.Fatal("a turn command is a command")
	}
	select {
	case <-turns.ran:
	case <-time.After(2 * time.Second):
		t.Fatal("the turn never ran")
	}
	got := turns.got
	if got.Message != "compare Go and Rust" || got.Agent != "research" || !got.UseHelpers || got.WorkspaceID != "ws" || got.ConversationID == "" {
		t.Errorf("turn request = %+v", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(conn.sentMessages()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sent := conn.sentMessages(); len(sent) != 2 || sent[0] != "a note" || sent[1] != "the answer" {
		t.Errorf("connector was sent %q, want the note then the answer", sent)
	}
}

// A refused turn command answers why and starts nothing.
func TestWebhookHandler_RefusedTurnCommandStartsNothing(t *testing.T) {
	turns := &fakeTurns{}
	conn := &sentTo{}
	comm := tools.NewCommunicationTools()
	comm.AddConnector("tg", "telegram", conn)
	h := &WebhookHandler{Commands: commands.New(turnCommand()), Turns: turns, CommTools: comm, Logger: logging.NewNopLogger()}

	h.handleCommand(inbound{workspaceID: "ws", connectorName: "tg", connectorType: "telegram"}, "/research")
	if sent := conn.sentMessages(); len(sent) != 1 || !strings.Contains(sent[0], "ask a question") {
		t.Errorf("connector was sent %q, want the refusal", sent)
	}
	if turns.got != nil {
		t.Errorf("a refused command started a turn: %+v", turns.got)
	}
}

// sentTo is a connector that records what it was asked to send. A command's turn sends from a goroutine, so reads go
// through sentMessages.
type sentTo struct {
	mu   sync.Mutex
	sent []string
}

func (s *sentTo) Send(_ context.Context, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, message)
	return nil
}

func (s *sentTo) sentMessages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
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

// startCommand is a /run-like command that "starts" the named automation and says so.
func startCommand() commands.Command {
	return commands.Command{
		Name: "run", Usage: "/run <automation>",
		Start: func(_ context.Context, r commands.Request) (commands.Started, error) {
			return commands.Started{Text: "Running '" + r.Args + "'...", Automation: r.Args}, nil
		},
	}
}

// /run from a connector: the command's line goes to the chat, then the automation's result when its run ends.
func TestWebhookHandler_RunCommandSendsTheAutomationsResult(t *testing.T) {
	tmp := t.TempDir()
	pm := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	if err := pm.WriteState("ws", &models.AgentState{LastRuns: map[string]*models.AutomationRun{"news": {Output: "the news report"}}}); err != nil {
		t.Fatal(err)
	}
	bus := eventbus.NewBus()
	conn := &sentTo{}
	comm := tools.NewCommunicationTools()
	comm.AddConnector("tg", "telegram", conn)
	h := &WebhookHandler{Commands: commands.New(startCommand()), AutomationEvents: bus, Persistence: pm, CommTools: comm, Logger: logging.NewNopLogger()}

	stream := assistant.AutomationStream("news")
	go func() { // the run ends once the handler is listening for it
		for bus.SubscriberCount("ws", stream) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		bus.Publish("ws", assistant.AgentEvent{Type: assistant.EventMessage, Automation: "news",
			Payload: proxy.Message{Role: proxy.AssistantRole, Content: assistant.MsgExecutionComplete}})
	}()
	if !h.handleCommand(inbound{workspaceID: "ws", connectorName: "tg", connectorType: "telegram"}, "/run news") {
		t.Fatal("/run is a command")
	}
	if sent := conn.sentMessages(); len(sent) != 2 || sent[0] != "Running 'news'..." || sent[1] != "the news report" {
		t.Errorf("connector was sent %q, want the start line then the report", sent)
	}
}
