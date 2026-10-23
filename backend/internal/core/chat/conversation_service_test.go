package chat

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/assistanttest"
	"llm-proxy/models"
)

type mockConvDeps struct {
	agent     *models.AgentConfig // nil: the adaptive result cap defaults to on
	modelName string
	guardrail *guardrails.GuardrailEngine
	store     *assistant.GuardrailDecisionStore
	events    assistant.EventPublisher
	log       logging.Logger

	memoryStore *memory.Store // nil unless a test seeds one
	memory      *models.MemoryConfig
	profiles    *profile.Catalog    // the built-ins unless a test sets its own
	modelCfg    *models.ModelConfig // nil: a small fixed config
	subRunner   assistant.SubRunner // nil: no delegation
}

func (m *mockConvDeps) Profiles() *profile.Catalog {
	if m.profiles == nil {
		m.profiles = profile.MustCatalog("", logging.NewNopLogger())
	}
	return m.profiles
}

func (m *mockConvDeps) SelectModels() (string, string) { return m.modelName, "" }
func (m *mockConvDeps) ModelConfig(string) (models.ModelConfig, bool) {
	if m.modelCfg != nil {
		return *m.modelCfg, true
	}
	return models.ModelConfig{MaxSteps: 2, MaxTokens: 100}, true
}
func (m *mockConvDeps) EffectiveToolCallFormat(context.Context, string) string    { return "" }
func (m *mockConvDeps) ProcessLogger(string) logging.Logger                       { return m.log }
func (m *mockConvDeps) GuardrailEngine() *guardrails.GuardrailEngine              { return m.guardrail }
func (m *mockConvDeps) GuardrailDecisionStore() *assistant.GuardrailDecisionStore { return m.store }
func (m *mockConvDeps) Orchestrator() *orchestrator.Orchestrator                  { return nil }
func (m *mockConvDeps) MemoryStore() *memory.Store                                { return m.memoryStore }
func (m *mockConvDeps) MemorySettings() *models.MemoryConfig                      { return m.memory }
func (m *mockConvDeps) AgentSettings() *models.AgentConfig                        { return m.agent }
func (m *mockConvDeps) SubRunner() assistant.SubRunner                            { return m.subRunner }
func (m *mockConvDeps) Events() assistant.EventPublisher                          { return m.events }
func (m *mockConvDeps) RunLoggingEnabled() bool                                   { return false }
func (m *mockConvDeps) RootDir() string                                           { return "" }

type mockEventPublisher struct {
	Published []assistant.AgentEvent
	Cleared   []string
}

func (m *mockEventPublisher) Publish(workspaceID string, event assistant.AgentEvent) {
	m.Published = append(m.Published, event)
}
func (m *mockEventPublisher) Clear(workspaceID string, channel assistant.EventChannel) {
	m.Cleared = append(m.Cleared, workspaceID)
}

// latestConversationID returns the conversation_id carried by the most recent
// lifecycle event, or "" when none was published. Used by tests to locate the
// persisted session for a run whose ID is generated server-side.
func (m *mockEventPublisher) latestConversationID() string {
	for i := len(m.Published) - 1; i >= 0; i-- {
		ev := m.Published[i]
		if ev.Type != assistant.EventLifecycle {
			continue
		}
		if p, ok := ev.Payload.(map[string]any); ok {
			if cid, ok := p["conversation_id"].(string); ok && cid != "" {
				return cid
			}
		}
	}
	return ""
}

func newMockConvDeps() *mockConvDeps {
	guardrailEngine := guardrails.NewGuardrailEngine(
		func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} },
		storage.NewPathResolver("", "", ""),
		nil,
		nil,
	)
	return &mockConvDeps{
		modelName: "test-model",
		guardrail: guardrailEngine,
		store:     assistant.NewGuardrailDecisionStore(),
		events:    &mockEventPublisher{},
		log:       logging.NewNopLogger(),
	}
}

func newTestPersistence(t *testing.T) *persistence.WorkspaceManager {
	t.Helper()
	tmp := t.TempDir()
	return persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
}

func TestConversationService_Execute_NewSession(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    proxy.AssistantRole,
						Content: "Here is a helpful response",
					}},
				},
			}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Reply != "Here is a helpful response" {
		t.Errorf("Reply = %q, want %q", result.Reply, "Here is a helpful response")
	}
	if result.WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want %q", result.WorkspaceID, "ws-1")
	}
	if result.ConversationID == "" {
		t.Error("ConversationID is empty")
	}
	if result.Canceled {
		t.Error("Canceled should be false")
	}
	if len(result.Events) == 0 {
		t.Error("Events should not be empty")
	}
}

func TestNormalizeConversationID(t *testing.T) {
	t.Run("empty generates fresh ID", func(t *testing.T) {
		got := NormalizeConversationID("")
		if !strings.HasPrefix(got, "conv_") || len(got) <= len("conv_") {
			t.Fatalf("NormalizeConversationID(\"\") = %q, want conv_<timestamp>", got)
		}
	})

	t.Run("non-empty preserved", func(t *testing.T) {
		const id = "conv_20260101120000"
		if got := NormalizeConversationID(id); got != id {
			t.Fatalf("NormalizeConversationID(%q) = %q, want %q", id, got, id)
		}
	})
}

func TestConversationService_Execute_EmptyWorkspaceID(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    proxy.AssistantRole,
						Content: "Here is a helpful response",
					}},
				},
			}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.WorkspaceID != "default" {
		t.Errorf("WorkspaceID = %q, want %q", result.WorkspaceID, "default")
	}
}

func TestConversationService_Execute_Canceled(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, context.Canceled
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Expected nil error for cancel, got: %v", err)
	}

	if !result.Canceled {
		t.Error("Canceled should be true")
	}
	if result.ConversationID == "" {
		t.Error("ConversationID missing in cancel result")
	}

	// A canceled run must still be persisted so a reloaded session shows the
	// user prompt and the cancel marker (the turn is stripped from LLM context
	// but retained for display).
	session, rErr := pm.ReadSession("ws-1", result.ConversationID)
	if rErr != nil {
		t.Fatalf("failed to read session after cancel: %v", rErr)
	}
	if session == nil {
		t.Fatal("session should exist after cancel")
	}
	if len(session.CancelledIndices) == 0 {
		t.Error("expected the canceled turn index to be recorded on the session")
	}
	if len(session.History) == 0 {
		t.Error("expected non-empty history to be persisted after cancel")
	}
}

// TestLastRunFailureText verifies the cancel-path failure salvage picks the
// most recent failure text (terminal error first, then upstream retry notices),
// so a reloaded cancelled session shows why the run was interrupted.
func TestLastRunFailureText(t *testing.T) {
	cases := []struct {
		name      string
		collected []assistant.AgentEvent
		want      string
	}{
		{
			name:      "no events",
			collected: nil,
			want:      "",
		},
		{
			name: "empty upstream has no failure",
			collected: []assistant.AgentEvent{
				{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{Event: "retry", Attempt: 1, MaxAttempts: 3}},
			},
			want: "",
		},
		{
			name: "upstream transport error surfaces",
			collected: []assistant.AgentEvent{
				{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: `Post "http://llama:8082": connection refused`, ErrClass: "connection-refused"}},
			},
			want: `Post "http://llama:8082": connection refused`,
		},
		{
			name: "terminal error wins over earlier upstream",
			collected: []assistant.AgentEvent{
				{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: "retry text"}},
				{Type: assistant.EventError, Payload: map[string]string{"error": "final failure"}},
			},
			want: "final failure",
		},
		{
			name: "most recent upstream wins over earlier ones",
			collected: []assistant.AgentEvent{
				{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: "first"}},
				{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 2, MaxAttempts: 3, Error: "second"}},
			},
			want: "second",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastRunFailureText(tc.collected); got != tc.want {
				t.Errorf("lastRunFailureText() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConversationService_CancelPersistsFailure verifies that a cancelled run
// that saw upstream failures persists the failure text as an assistant error
// message, so a reloaded session renders why the run was interrupted instead
// of a bare "Response interrupted" marker.
func TestConversationService_CancelPersistsFailure(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	session := &models.AssistantSession{
		ID:          "conv_cancel_fail",
		WorkspaceID: "ws-1",
		Timezone:    "UTC",
		History: []proxy.Message{
			{Role: proxy.SystemRole, Content: "system"},
			{Role: proxy.UserRole, Content: "list files"},
		},
	}
	if err := pm.WriteSession("ws-1", session); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	collected := []assistant.AgentEvent{
		{Type: assistant.EventUpstream, Payload: assistant.UpstreamEventPayload{
			Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3,
			Error:    `Post "http://192.168.50.60:8082/v1/chat/completions": dial tcp: connect: connection refused`,
			ErrClass: "connection-refused",
		}},
	}
	pub := &mockEventPublisher{}

	// handleCancelResult lives on the concrete service; the public API only
	// exposes Execute. Drive the cancel path directly via the concrete type.
	concrete, ok := svc.(*conversationService)
	if !ok {
		t.Fatal("NewConversationService did not return *conversationService")
	}
	concrete.handleCancelResult(session, "ws-1", "", nil, nil, pub, collected, logging.NewNopLogger(), context.Canceled)

	// The failure must be persisted as an assistant-role message carrying the
	// Error field (buildSegmentsFromHistory renders it as an error segment).
	persisted, rErr := pm.ReadSession("ws-1", "conv_cancel_fail")
	if rErr != nil {
		t.Fatalf("read persisted session: %v", rErr)
	}
	if persisted == nil {
		t.Fatal("session should exist after cancel")
	}
	var foundError bool
	for _, m := range persisted.History {
		if m.Role == proxy.AssistantRole && m.Error != "" {
			foundError = true
			if !strings.Contains(m.Error, "connection refused") {
				t.Errorf("persisted error = %q, want it to mention the upstream failure", m.Error)
			}
		}
	}
	if !foundError {
		t.Error("expected a persisted assistant error message after cancel with upstream failure, got none")
	}

	// The cancelled user turn must still be marked cancelled so the UI shows
	// the "Response interrupted" banner alongside the error.
	if len(persisted.CancelledIndices) == 0 {
		t.Error("expected the cancelled user turn index to be recorded")
	}

	// The failure text must not leak into the next run's LLM context: the
	// cancelled turn (user + error marker) is stripped by FilterCancelledTurns.
	filtered := FilterCancelledTurns(persisted.History, persisted.CancelledIndices)
	for _, m := range filtered {
		if m.Error != "" {
			t.Errorf("error message leaked into filtered LLM context: %q", m.Error)
		}
	}
}

func TestConversationService_Execute_AgentError(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, errors.New("model unavailable")
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	_, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err == nil {
		t.Fatal("expected error from agent execution")
	}

	// A terminal mid-run upstream failure must surface to the UI: an error
	// event (the frontend renders an error segment + clears loading) and a
	// session_completed lifecycle so the sidebar stops showing it as running.
	var gotErr bool
	var gotCompleted bool
	for _, ev := range pub.Published {
		if ev.Type == assistant.EventError {
			gotErr = true
			if _, ok := ev.Payload.(map[string]string); !ok {
				t.Errorf("error event payload should be map[string]string, got %T", ev.Payload)
			}
		}
		if ev.Type == assistant.EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == assistant.PhaseSessionCompleted {
				gotCompleted = true
			}
		}
	}
	if !gotErr {
		t.Error("expected an EventError published on terminal failure, got none")
	}
	if !gotCompleted {
		t.Error("expected session_completed lifecycle published on terminal failure, got none")
	}

	// The failure must be persisted so a reloaded session shows why it stopped
	// instead of a turn with only the user prompt. The error is stored as an
	// assistant-role message carrying the Error field.
	convID := pub.latestConversationID()
	session, rErr := pm.ReadSession("ws-1", convID)
	if rErr != nil {
		t.Fatalf("failed to read session after error: %v", rErr)
	}
	if session == nil {
		t.Fatal("session should exist after a terminal failure")
	}
	var foundError bool
	for _, m := range session.History {
		if m.Role == proxy.AssistantRole && m.Error != "" {
			foundError = true
			if !strings.Contains(m.Error, "model unavailable") {
				t.Errorf("persisted error = %q, want it to mention the run failure", m.Error)
			}
		}
	}
	if !foundError {
		t.Error("expected an assistant-role error message persisted in session history")
	}
}

func TestConversationService_Execute_ExistingSession(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	sessionID := "conv-test-existing"
	existing := &models.AssistantSession{
		ID:          sessionID,
		WorkspaceID: "ws-1",
		History: []proxy.Message{
			{Role: proxy.SystemRole, Content: "system"},
			{Role: proxy.UserRole, Content: "first message"},
			{Role: proxy.AssistantRole, Content: "first reply"},
		},
	}
	if err := pm.WriteSession("ws-1", existing); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}

	callCount := 0
	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{
						{Message: proxy.Message{
							Role:    proxy.AssistantRole,
							Content: "Here is a helpful response",
						}},
					},
				}, nil
			}
			// This shouldn't be hit since no tools means one-turn completion
			return nil, errors.New("unexpected call")
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: sessionID, Message: "second message", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ConversationID != sessionID {
		t.Errorf("ConversationID = %q, want %q", result.ConversationID, sessionID)
	}

	readSession, rErr := pm.ReadSession("ws-1", sessionID)
	if rErr != nil {
		t.Fatalf("failed to read session: %v", rErr)
	}
	if readSession == nil {
		t.Fatal("session should exist after Execute")
	}
	// History should have system + first user + first assistant + second user + second assistant
	if len(readSession.History) < 4 {
		t.Errorf("history too short: got %d, want >= 4", len(readSession.History))
	}
}

func TestConversationService_Execute_ExcludeTools(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    proxy.AssistantRole,
						Content: "Here is a helpful response",
					}},
				},
			}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: []string{"tool-a", "tool-b"}, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Execute with exclude tools failed: %v", err)
	}
	if result.ConversationID == "" {
		t.Fatal("ConversationID is empty")
	}
}

func TestConversationService_Execute_SessionPersisted(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    proxy.AssistantRole,
						Content: "Here is a helpful response",
					}},
				},
			}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-persist", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	convID := result.ConversationID

	session, rErr := pm.ReadSession("ws-persist", convID)
	if rErr != nil {
		t.Fatalf("failed to read persisted session: %v", rErr)
	}
	if session == nil {
		t.Fatal("session not persisted")
	}
	if session.WorkspaceID != "ws-persist" {
		t.Errorf("workspace_id = %q, want %q", session.WorkspaceID, "ws-persist")
	}
	if len(session.History) == 0 {
		t.Error("history should not be empty")
	}
}

// TestBuildObserver_CheckpointsEveryToolResult verifies the documented contract
// (session-source-backend-driven.md): every tool result triggers an
// unconditional checkpoint write — no throttling. Back-to-back tool results
// (well within the historical 1s throttle window) must each be reflected in the
// persisted session, so a refresh / sidebar click on a running session returns
// the latest committed tool calls.
func TestBuildObserver_CheckpointsEveryToolResult(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := &conversationService{deps: deps, persistence: pm}

	base := []proxy.Message{
		{Role: proxy.SystemRole, Content: "sys"},
		{Role: proxy.UserRole, Content: "task"},
	}
	session := &models.AssistantSession{
		ID:          "conv-checkpoint",
		WorkspaceID: "ws-checkpoint",
		History:     base,
	}
	if err := pm.WriteSession("ws-checkpoint", session); err != nil {
		t.Fatalf("write initial session: %v", err)
	}

	pub := &mockEventPublisher{}
	obs, _ := svc.buildObserver(base, session, "ws-checkpoint", pub, nil, logging.NewNopLogger())

	for i := 1; i <= 3; i++ {
		obs(assistant.AgentEvent{
			Type: assistant.EventToolResult,
			Payload: map[string]any{
				"id":     fmt.Sprintf("tool-%d", i),
				"result": fmt.Sprintf("result-%d", i),
			},
		})

		persisted, rErr := pm.ReadSession("ws-checkpoint", "conv-checkpoint")
		if rErr != nil {
			t.Fatalf("read session after event %d: %v", i, rErr)
		}
		if persisted == nil {
			t.Fatalf("session not persisted after event %d", i)
		}
		toolMsgs := 0
		for _, m := range persisted.History {
			if m.Role == proxy.ToolRole {
				toolMsgs++
			}
		}
		if toolMsgs != i {
			t.Fatalf("after %d tool results, expected %d tool messages in checkpoint, got %d", i, i, toolMsgs)
		}
	}
}

// The run log the checkpoints are built from keeps only the newest snapshot of a
// stream: reasoning and tool_stream carry the text so far, and keeping each one
// made the log (and its rescan on every tool result) grow with every token.
func TestBuildObserver_KeepsNewestSnapshotPerStream(t *testing.T) {
	svc := &conversationService{deps: newMockConvDeps(), persistence: newTestPersistence(t)}
	base := []proxy.Message{{Role: proxy.UserRole, Content: "task"}}
	session := &models.AssistantSession{ID: "conv-snap", WorkspaceID: "ws-snap", History: base}

	obs, collected := svc.buildObserver(base, session, "ws-snap", &mockEventPublisher{}, nil, logging.NewNopLogger())
	for i := 1; i <= 1000; i++ {
		obs(assistant.AgentEvent{Type: assistant.EventReasoning, ConversationID: "conv-snap", Payload: strings.Repeat("r", i)})
	}
	obs(assistant.AgentEvent{Type: assistant.EventStepStart, ConversationID: "conv-snap", Payload: map[string]int{"step": 2}})
	obs(assistant.AgentEvent{Type: assistant.EventReasoning, ConversationID: "conv-snap", Payload: "second request"})

	events := collected()
	if len(events) != 3 {
		t.Fatalf("run log has %d events, want 3 (newest reasoning, step, reasoning)", len(events))
	}
	if got := events[0].Payload.(string); len(got) != 1000 {
		t.Errorf("first request kept a %d-char snapshot, want the newest (1000)", len(got))
	}
}

func TestSupersedesSnapshot(t *testing.T) {
	text := func(typ assistant.AgentEventType, conv string) assistant.AgentEvent {
		return assistant.AgentEvent{Type: typ, ConversationID: conv, Payload: "so far"}
	}
	tests := []struct {
		name       string
		prev, next assistant.AgentEvent
		want       bool
	}{
		{"same stream", text(assistant.EventReasoning, "c1"), text(assistant.EventReasoning, "c1"), true},
		{"content stream", text(assistant.EventToolStream, "c1"), text(assistant.EventToolStream, "c1"), true},
		{"other conversation", text(assistant.EventReasoning, "c1"), text(assistant.EventReasoning, "c2"), false},
		{"no conversation id", text(assistant.EventReasoning, ""), text(assistant.EventReasoning, ""), false},
		{"other type", text(assistant.EventReasoning, "c1"), text(assistant.EventToolStream, "c1"), false},
		{"not a snapshot type", text(assistant.EventMessage, "c1"), text(assistant.EventMessage, "c1"), false},
		{"structured payload", assistant.AgentEvent{Type: assistant.EventToolStream, ConversationID: "c1", Payload: map[string]any{"a": 1}}, text(assistant.EventToolStream, "c1"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := assistant.SupersedesSnapshot(tc.prev, tc.next); got != tc.want {
				t.Errorf("SupersedesSnapshot = %v, want %v", got, tc.want)
			}
		})
	}
}

// orderedEventPublisher records the sequence of Clear and Publish operations so
// a test can assert ordering between the recent-buffer clear (setupRun) and the
// session_started lifecycle publish. It mirrors the EventBus semantics (Clear
// wipes the `recent` replay buffer) without importing automation (which would
// create an import cycle from the assistant test package).
type orderedEventPublisher struct {
	ops       []string
	published []assistant.AgentEvent
}

func (o *orderedEventPublisher) Publish(workspaceID string, event assistant.AgentEvent) {
	o.published = append(o.published, event)
	if event.Type == assistant.EventLifecycle {
		if p, ok := event.Payload.(map[string]any); ok && p["phase"] == assistant.PhaseSessionStarted {
			o.ops = append(o.ops, "publish:session_started")
			return
		}
	}
	o.ops = append(o.ops, "publish")
}

func (o *orderedEventPublisher) Clear(workspaceID string, channel assistant.EventChannel) {
	o.ops = append(o.ops, "clear")
}

// TestExecute_SessionStartedPublishedAfterClear verifies the reorder that keeps
// session_started in the `recent` replay buffer for the current run: setupRun
// clears `recent` (to drop stale events from prior runs), and session_started is
// published AFTER that clear so a refreshed / reopened tab can reconstruct the
// running turn from the replay, anchored by this event and its user snippet.
func TestExecute_SessionStartedPublishedAfterClear(t *testing.T) {
	deps := newMockConvDeps()
	pm := newTestPersistence(t)
	svc := &conversationService{deps: deps, persistence: pm}

	client := &assistanttest.MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: proxy.AssistantRole, Content: "ok"}},
				},
			}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{}}
	engine := &assistanttest.MockEngine{Result: "ok"}
	pub := &orderedEventPublisher{}
	log := logging.NewNopLogger()

	if _, err := svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: log, Provider: provider, Client: client, Engine: engine, Events: pub, Recorder: nil}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	startedIdx, clearIdx := -1, -1
	for i, op := range pub.ops {
		switch op {
		case "publish:session_started":
			startedIdx = i
		case "clear":
			if clearIdx < 0 {
				clearIdx = i
			}
		}
	}
	if startedIdx < 0 {
		t.Fatal("session_started was never published")
	}
	if clearIdx < 0 {
		t.Fatal("recent buffer was never cleared (setupRun)")
	}
	if startedIdx < clearIdx {
		t.Errorf("session_started published at %d BEFORE recent clear at %d — it would be dropped from replay", startedIdx, clearIdx)
	}
}

// The assistant must hand the memory store to its agent: hot memory is only
// injected (and usage only counted) when the agent has a store. Before this
// test, buildAgent enabled hot memory but never passed the store, so chats got
// no memory at all while the unit tests of the agent itself stayed green.
// Whether a chat gets it follows the workspace override, else the global default.
func TestConversationService_Execute_HotMemoryFollowsOverrides(t *testing.T) {
	cases := []struct {
		name      string
		global    *models.MemoryConfig
		workspace models.MemoryMode
		wantFact  bool
	}{
		{"shipped default is on", nil, models.MemoryModeInherit, true},
		{"global off", &models.MemoryConfig{AssistantHot: new(false)}, models.MemoryModeInherit, false},
		{"workspace off beats global on", &models.MemoryConfig{AssistantHot: new(true)}, models.MemoryModeOff, false},
		{"workspace on beats global off", &models.MemoryConfig{AssistantHot: new(false)}, models.MemoryModeOn, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestMemoryStore(t)
			ctx := context.Background()
			if _, err := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "build", Content: "run go build ./... to verify", Tags: []string{memory.HotTag}, Source: "agent"}); err != nil {
				t.Fatal(err)
			}
			deps := newMockConvDeps()
			deps.memoryStore = store
			deps.memory = tc.global
			pm := newTestPersistence(t)
			if err := pm.WriteConfig("ws-1", &models.WorkspaceConfig{AssistantMemory: tc.workspace}); err != nil {
				t.Fatal(err)
			}
			svc := NewConversationService(deps, pm)

			var head string
			client := &assistanttest.MockClient{
				StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
					if head == "" && len(req.Messages) > 0 {
						head = req.Messages[0].Content
					}
					ch := make(chan *proxy.ChatResponse, 1)
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Here is a helpful response"}}}}
					close(ch)
					return ch, nil
				},
			}
			if _, err := svc.Execute(ctx, ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "", Message: "how do I build?", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{Tools: []proxy.Tool{}}, Client: client, Engine: &assistanttest.MockEngine{Result: "ok"}, Events: &mockEventPublisher{}, Recorder: nil}); err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if got := strings.Contains(head, "<memory>") && strings.Contains(head, "go build"); got != tc.wantFact {
				t.Fatalf("hot memory in the head message = %v, want %v: %q", got, tc.wantFact, head)
			}
			if err := store.FlushUsage(ctx); err != nil {
				t.Fatal(err)
			}
			all, _ := store.List(ctx, "ws-1", "", 10, 0)
			wantCount := 0
			if tc.wantFact {
				wantCount = 1
			}
			if len(all) != 1 || all[0].InjectedCount != wantCount {
				t.Errorf("usage count = %+v, want %d", all, wantCount)
			}
		})
	}
}

// Every turn records how it ran on its user message — the model, when it
// started and how long it took — whether it finishes, fails or is cancelled,
// so a reloaded conversation can say "Worked 42s · model". Earlier turns keep
// their record when later turns run.
func TestConversationService_RecordsTurnRun(t *testing.T) {
	answer := func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
		return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "Here is a helpful response"}}}}, nil
	}
	cases := []struct {
		name string
		chat func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error)
	}{
		{"finished", answer},
		{"failed", func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, errors.New("model unavailable")
		}},
		{"cancelled", func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) { return nil, context.Canceled }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pm := newTestPersistence(t)
			svc := NewConversationService(newMockConvDeps(), pm)
			client := &assistanttest.MockClient{
				StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
					return nil, errors.New("no stream")
				},
				ChatFunc: tc.chat,
			}
			before := time.Now()
			_, _ = svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv_run", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{}, Client: client, Engine: &assistanttest.MockEngine{}, Events: &mockEventPublisher{}, Recorder: nil})

			session, err := pm.ReadSession("ws-1", "conv_run")
			if err != nil || session == nil {
				t.Fatalf("ReadSession: %v, %v", session, err)
			}
			run := turnRunOf(t, session.History, "hello")
			if run.Model != "test-model" {
				t.Errorf("model = %q, want test-model", run.Model)
			}
			if run.StartedAt.Before(before.Add(-time.Second)) || run.DurationMs < 0 {
				t.Errorf("run = %+v, want a start time from this run and a duration", run)
			}
		})
	}

	t.Run("an earlier turn keeps its record", func(t *testing.T) {
		pm := newTestPersistence(t)
		svc := NewConversationService(newMockConvDeps(), pm)
		client := &assistanttest.MockClient{
			StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
				return nil, errors.New("no stream")
			},
			ChatFunc: answer,
		}
		for _, msg := range []string{"first", "second"} {
			_, _ = svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv_two", Message: msg, ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{}, Client: client, Engine: &assistanttest.MockEngine{}, Events: &mockEventPublisher{}, Recorder: nil})
		}
		session, _ := pm.ReadSession("ws-1", "conv_two")
		turnRunOf(t, session.History, "first")
		turnRunOf(t, session.History, "second")
	})
}

func turnRunOf(t *testing.T, history []proxy.Message, userText string) *models.TurnRun {
	t.Helper()
	for _, m := range history {
		if m.Role == proxy.UserRole && m.Content == userText {
			if m.Run == nil {
				t.Fatalf("user message %q has no run record", userText)
			}
			return m.Run
		}
	}
	t.Fatalf("user message %q not in history", userText)
	return nil
}

// A provider's reported token counts land on the turn's run record.
func TestConversationService_RecordsReportedTokens(t *testing.T) {
	pm := newTestPersistence(t)
	svc := NewConversationService(newMockConvDeps(), pm)
	client := &assistanttest.MockClient{
		StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "Here is a helpful response"}}},
				Usage:   &proxy.TokenUsage{PromptTokens: 1200, CompletionTokens: 85},
			}, nil
		},
	}
	_, _ = svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv_tok", Message: "hello", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{}, Client: client, Engine: &assistanttest.MockEngine{}, Events: &mockEventPublisher{}, Recorder: nil})
	session, _ := pm.ReadSession("ws-1", "conv_tok")
	run := turnRunOf(t, session.History, "hello")
	if run.PromptTokens != 1200 || run.CompletionTokens != 85 {
		t.Errorf("run tokens = %d/%d, want 1200/85", run.PromptTokens, run.CompletionTokens)
	}
}

// A tool that became unavailable during a chat turn is recorded on that turn's run record, so the transcript shows it
// after a reload even if the model's answer never mentions it.
func TestConversationService_RecordsToolWarningsOnTheTurn(t *testing.T) {
	pm := newTestPersistence(t)
	svc := NewConversationService(newMockConvDeps(), pm)
	calls := 0
	client := &assistanttest.MockClient{
		StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
			calls++
			if calls == 1 {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole,
					ToolCalls: []proxy.ToolCall{{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "web_lookup", Arguments: `{"q":"x"}`}}}}}}}, nil
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "Here is what I found."}}}}, nil
		},
	}
	provider := &assistanttest.MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "web_lookup"}}}}
	engine := &assistanttest.MockEngine{Err: fmt.Errorf("key rejected: %w", models.ErrToolUnavailable)}
	_, _ = svc.Execute(context.Background(), ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv_warn", Message: "look it up", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: provider, Client: client, Engine: engine, Events: &mockEventPublisher{}, Recorder: nil})

	session, err := pm.ReadSession("ws-1", "conv_warn")
	if err != nil || session == nil {
		t.Fatalf("ReadSession: %v, %v", session, err)
	}
	run := turnRunOf(t, session.History, "look it up")
	if len(run.Warnings) != 1 || !strings.Contains(run.Warnings[0], "web_lookup unavailable") {
		t.Fatalf("turn warnings = %v, want one 'web_lookup unavailable' warning", run.Warnings)
	}
}

// A message may name its own model; the run, its record and its agent use that one, not the primary.
func TestConversationService_ResolveModelConfig_HonoursTheModelOverride(t *testing.T) {
	deps := newMockConvDeps()
	deps.modelName = "primary"
	svc := NewConversationService(deps, newTestPersistence(t)).(*conversationService)

	if got, _ := svc.resolveModelConfig(context.Background(), "ws", "c1", "hi"); got != "primary" {
		t.Errorf("no override: model = %q, want primary", got)
	}
	ctx := models.WithModelOverride(context.Background(), "other")
	if got, _ := svc.resolveModelConfig(ctx, "ws", "c1", "hi"); got != "other" {
		t.Errorf("override: model = %q, want other", got)
	}
}

// chatTurns runs messages through Execute one after another on a fresh service and returns, per turn, the system
// message the model was sent and the session's stored profile afterwards.
type chatTurnResult struct{ system, storedProfile string }

// chatTurnSpec is one turn of chatTurnsWith: its context (a profile request) and its one-message agent.
type chatTurnSpec struct {
	ctx   context.Context
	agent string
}

func chatTurns(t *testing.T, pm *persistence.WorkspaceManager, turns []context.Context) []chatTurnResult {
	t.Helper()
	specs := make([]chatTurnSpec, len(turns))
	for i, ctx := range turns {
		specs[i] = chatTurnSpec{ctx: ctx}
	}
	return chatTurnsWith(t, pm, specs)
}

func chatTurnsWith(t *testing.T, pm *persistence.WorkspaceManager, turns []chatTurnSpec) []chatTurnResult {
	t.Helper()
	svc := NewConversationService(newMockConvDeps(), pm)
	var sent string
	client := &assistanttest.MockClient{
		StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(_ context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			sent = req.Messages[0].Content
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: proxy.AssistantRole, Content: "Here is a helpful response"}}}}, nil
		},
	}
	var out []chatTurnResult
	for _, turn := range turns {
		if _, err := svc.Execute(turn.ctx, ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv-profiles", Message: "a question", ContextVersion: "v1", Timezone: "UTC", Agent: turn.agent, ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{}, Client: client, Engine: &assistanttest.MockEngine{Result: "ok"}, Events: &mockEventPublisher{}, Recorder: nil}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		session, err := pm.ReadSession("ws-1", "conv-profiles")
		if err != nil || session == nil {
			t.Fatalf("ReadSession: %v", err)
		}
		out = append(out, chatTurnResult{system: sent, storedProfile: session.Profile})
	}
	return out
}

// A chat starts as General, a message may ask for another profile and the conversation then keeps it, a later
// message can switch again, and every turn is sent the current profile's behaviour.
func TestConversationService_Execute_ProfileChoiceAndPerTurnPrompt(t *testing.T) {
	pm := newTestPersistence(t)
	bg := context.Background()
	research := models.WithProfileRequest(bg, "research")
	got := chatTurns(t, pm, []context.Context{bg, research, bg, models.WithProfileRequest(bg, "builder")})

	wantMarker := []string{"helpful, capable assistant", "find out what is true", "find out what is true", "read, write and fix code"}
	wantStored := []string{"", "research", "research", "builder"}
	for i, turn := range got {
		if !strings.Contains(turn.system, wantMarker[i]) {
			t.Errorf("turn %d: the system message should carry %q:\n%s", i+1, wantMarker[i], turn.system)
		}
		if turn.storedProfile != wantStored[i] {
			t.Errorf("turn %d: stored profile = %q, want %q", i+1, turn.storedProfile, wantStored[i])
		}
	}
}

// A one-message agent (a turn command such as /research) runs that turn under its profile and leaves the
// conversation's own profile as it was, so the next plain message runs under the conversation's choice again.
func TestConversationService_Execute_OneMessageAgent(t *testing.T) {
	pm := newTestPersistence(t)
	bg := context.Background()
	got := chatTurnsWith(t, pm, []chatTurnSpec{
		{ctx: bg},
		{ctx: bg, agent: "research"},
		{ctx: bg},
		{ctx: models.WithProfileRequest(bg, "builder")},
		{ctx: bg, agent: "research"},
		{ctx: bg},
	})
	wantMarker := []string{"helpful, capable assistant", "find out what is true", "helpful, capable assistant",
		"read, write and fix code", "find out what is true", "read, write and fix code"}
	wantStored := []string{"", "", "", "builder", "builder", "builder"}
	for i, turn := range got {
		if !strings.Contains(turn.system, wantMarker[i]) {
			t.Errorf("turn %d: the system message should carry %q:\n%s", i+1, wantMarker[i], turn.system)
		}
		if turn.storedProfile != wantStored[i] {
			t.Errorf("turn %d: stored profile = %q, want %q", i+1, turn.storedProfile, wantStored[i])
		}
	}
}

// The workspace's profile is the start for its chats; a conversation's own choice outranks it.
func TestConversationService_Execute_WorkspaceDefaultProfile(t *testing.T) {
	pm := newTestPersistence(t)
	if err := pm.WriteConfig("ws-1", &models.WorkspaceConfig{AssistantProfile: "operator"}); err != nil {
		t.Fatal(err)
	}
	got := chatTurns(t, pm, []context.Context{context.Background(), models.WithProfileRequest(context.Background(), "research")})
	if !strings.Contains(got[0].system, "machine or network") {
		t.Errorf("turn 1 should use the workspace's Operator profile:\n%s", got[0].system)
	}
	if !strings.Contains(got[1].system, "find out what is true") {
		t.Errorf("turn 2: the conversation's own choice outranks the workspace's:\n%s", got[1].system)
	}
}

// A conversation that began before profiles carries the old persona in its stored system message; the next turn
// replaces it, so the fix reaches existing chats.
func TestConversationService_Execute_RefreshesAStoredSystemMessage(t *testing.T) {
	pm := newTestPersistence(t)
	stale := &models.AssistantSession{ID: "conv-profiles", WorkspaceID: "ws-1", History: []proxy.Message{
		{Role: proxy.SystemRole, Content: "You are an autonomous agent... BEST PRACTICE: Always start by verifying your environment."},
		{Role: proxy.UserRole, Content: "earlier"},
		{Role: proxy.AssistantRole, Content: "earlier answer"},
	}}
	if err := pm.WriteSession("ws-1", stale); err != nil {
		t.Fatal(err)
	}
	got := chatTurns(t, pm, []context.Context{context.Background()})
	if strings.Contains(got[0].system, "verifying your environment") || !strings.Contains(got[0].system, "helpful, capable assistant") {
		t.Errorf("the stored persona must be replaced by the current profile:\n%s", got[0].system)
	}
	session, _ := pm.ReadSession("ws-1", "conv-profiles")
	if strings.Contains(session.History[0].Content, "verifying your environment") {
		t.Error("the refreshed system message is what gets persisted")
	}
}

// optionsView is the part of AgentOptions a run's settings decide, in a comparable form: pointers and functions are
// reduced to whether they are set. The characterisation tests below record what each kind of run resolves today, so
// a refactor of how options are built has to keep them.
type optionsView struct {
	MaxSteps, ContextBudget, MaxResponseTokens, ReasoningBudget, MaxPlanSteps int
	ReasoningEnabled, UseNativeTools                                          string
	Temperature                                                               float64
	WorkspaceID, ModelName, ProviderType, ConversationID, ProfileID           string
	GuardrailTimeoutBehavior                                                  string
	WorkloadClass                                                             models.WorkloadClass
	LoopStrategy                                                              assistant.LoopStrategyName
	Channel                                                                   assistant.EventChannel
	RunNetworkScope                                                           models.NetworkScope
	UsePrefill, EnableHotMemory, AdaptiveResultCap                            bool
	HasMemoryStore, HasGuardrails, HasObserver, HasDecisionHandler            bool
	HasLogger, HasOrchestrator                                                bool
	AllowedTools, ExcludedTools                                               []string
	GlobalTimeout, ToolTimeout, FilesystemToolTimeout, MaxPlanDuration        time.Duration
	GuardrailTimeout, GuardrailApprovalTimeout                                time.Duration
}

func boolView(b *bool) string {
	if b == nil {
		return "unset"
	}
	return fmt.Sprint(*b)
}

func viewOptions(o assistant.AgentOptions) optionsView {
	return optionsView{
		MaxSteps: o.MaxSteps, ContextBudget: o.ContextBudget, MaxResponseTokens: o.MaxResponseTokens,
		ReasoningBudget: o.ReasoningBudget, MaxPlanSteps: o.MaxPlanSteps,
		ReasoningEnabled: boolView(o.ReasoningEnabled), UseNativeTools: boolView(o.UseNativeTools),
		Temperature: o.Temperature, WorkspaceID: o.WorkspaceID, ModelName: o.ModelName, ProviderType: o.ProviderType,
		ConversationID: o.ConversationID, ProfileID: o.Profile.ID, GuardrailTimeoutBehavior: o.GuardrailTimeoutBehavior,
		WorkloadClass: o.WorkloadClass, LoopStrategy: o.LoopStrategy, Channel: o.Channel, RunNetworkScope: o.RunNetworkScope,
		UsePrefill: o.UsePrefill, EnableHotMemory: o.EnableHotMemory, AdaptiveResultCap: o.AdaptiveResultCap,
		HasMemoryStore: o.MemoryStore != nil, HasGuardrails: o.Guardrails != nil, HasObserver: o.Observer != nil,
		HasDecisionHandler: o.GuardrailDecisionHandler != nil, HasLogger: o.Logger != nil, HasOrchestrator: o.Orchestrator != nil,
		AllowedTools: o.AllowedTools, ExcludedTools: o.ExcludedTools,
		GlobalTimeout: o.GlobalTimeout, ToolTimeout: o.ToolTimeout, FilesystemToolTimeout: o.FilesystemToolTimeout,
		MaxPlanDuration: o.MaxPlanDuration, GuardrailTimeout: o.GuardrailTimeout, GuardrailApprovalTimeout: o.GuardrailApprovalTimeout,
	}
}

// fullModelConfig sets every field ApplyModelConfig reads, so a resolver that drops one shows up.
func fullModelConfig(name string) models.ModelConfig {
	return models.ModelConfig{
		Name: name, Provider: "openai", WorkloadClass: models.WorkloadCloud, LoopStrategy: models.LoopStrategyPlanExecute,
		MaxSteps: 21, ContextBudget: 9000, MaxTokens: 1500, ReasoningBudget: 700, ReasoningEnabled: new(true),
		Temperature: 0.3, ToolCallFormat: "native", Prefill: new(true), TimeoutMinutes: 7, ToolTimeoutSeconds: 40,
		FilesystemToolTimeoutSeconds: 9, MaxPlanDurationMinutes: 11, MaxPlanSteps: 13, GuardrailTimeoutSeconds: 3,
		GuardrailTimeoutBehavior: "fail-closed", GuardrailApprovalTimeoutSecs: 90,
	}
}

// fullModelView is what fullModelConfig resolves to in any kind of run.
func fullModelView(v optionsView) optionsView {
	v.MaxSteps, v.ContextBudget, v.MaxResponseTokens, v.ReasoningBudget, v.MaxPlanSteps = 21, 9000, 1500, 700, 13
	v.ReasoningEnabled, v.UseNativeTools, v.Temperature, v.UsePrefill = "true", "true", 0.3, true
	v.ProviderType, v.WorkloadClass, v.LoopStrategy, v.GuardrailTimeoutBehavior = "openai", models.WorkloadCloud, assistant.LoopPlanExecute, "fail-closed"
	v.GlobalTimeout, v.ToolTimeout, v.FilesystemToolTimeout = 7*time.Minute, 40*time.Second, 9*time.Second
	v.MaxPlanDuration, v.GuardrailTimeout, v.GuardrailApprovalTimeout = 11*time.Minute, 3*time.Second, 90*time.Second
	return v
}

// A chat turn always carries the memory store (memory_search needs it even with hot memory off), its conversation
// id, a deny list and no network scope of its own.
func TestChatAgentOptions_Characterisation(t *testing.T) {
	research := profile.MustCatalog("", logging.NewNopLogger()).Resolve(profile.KindAssistant, "research")
	general := profile.MustCatalog("", logging.NewNopLogger()).Resolve(profile.KindAssistant)
	full := fullModelConfig("cloud-m")
	base := optionsView{
		MaxSteps: 2, MaxResponseTokens: 100, ReasoningEnabled: "unset", UseNativeTools: "unset",
		WorkspaceID: "ws-1", ModelName: "cloud-m", ConversationID: "conv-1", ProfileID: "general", Channel: assistant.ChannelAssistant,
		EnableHotMemory: true, AdaptiveResultCap: true, HasMemoryStore: true, HasGuardrails: true, HasObserver: true,
		HasDecisionHandler: true, HasLogger: true,
	}
	cases := []struct {
		name      string
		modelCfg  *models.ModelConfig
		memory    *models.MemoryConfig
		agent     *models.AgentConfig
		workspace models.MemoryMode
		profile   profile.Profile
		exclude   []string
		want      func(optionsView) optionsView
	}{
		{name: "defaults", profile: general, want: func(v optionsView) optionsView { return v }},
		{name: "full model config", modelCfg: &full, profile: general, want: fullModelView},
		{name: "research profile, workspace memory off", profile: research, workspace: models.MemoryModeOff, exclude: []string{models.ToolTerminalExecute},
			want: func(v optionsView) optionsView {
				v.ProfileID, v.EnableHotMemory, v.ExcludedTools = "research", false, []string{models.ToolTerminalExecute}
				return v
			}},
		{name: "global memory off, result cap off", profile: general, memory: &models.MemoryConfig{AssistantHot: new(false)}, agent: &models.AgentConfig{AdaptiveResultCap: new(false)},
			want: func(v optionsView) optionsView {
				v.EnableHotMemory, v.AdaptiveResultCap = false, false
				return v
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newMockConvDeps()
			deps.modelCfg, deps.memory, deps.agent = tc.modelCfg, tc.memory, tc.agent
			deps.memoryStore = newTestMemoryStore(t)
			pm := newTestPersistence(t)
			if err := pm.WriteConfig("ws-1", &models.WorkspaceConfig{AssistantMemory: tc.workspace}); err != nil {
				t.Fatal(err)
			}
			svc := NewConversationService(deps, pm).(*conversationService)
			got := viewOptions(svc.agentOptions(chatAgentRun{
				modelName: "cloud-m", workspaceID: "ws-1", sessionID: "conv-1", log: logging.NewNopLogger(),
				observer: func(assistant.AgentEvent) {}, excludeTools: tc.exclude, profile: tc.profile,
			}))
			if want := tc.want(base); !reflect.DeepEqual(got, want) {
				t.Errorf("resolved options changed\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

// offSubRunner is a delegation package whose helpers are off.
type offSubRunner struct{}

func (offSubRunner) Enabled() bool                    { return false }
func (offSubRunner) Problem() assistant.HelperProblem { return assistant.HelperNotCloud }
func (offSubRunner) Run(context.Context, assistant.SubRunRequest) assistant.SubRunResult {
	return assistant.SubRunResult{}
}

// fakeSubRunner stands in for the delegation package: it records each task, reports progress like a child run and
// answers with a summary.
type fakeSubRunner struct {
	mu            sync.Mutex
	requests      []assistant.SubRunRequest
	shortDeadline bool // a task ran under the per-tool timeout, which a multi-minute child would hit
}

func (f *fakeSubRunner) Enabled() bool                    { return true }
func (f *fakeSubRunner) Problem() assistant.HelperProblem { return assistant.HelperNone }

func (f *fakeSubRunner) Run(ctx context.Context, req assistant.SubRunRequest) assistant.SubRunResult {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= assistant.DefaultToolTimeout {
		f.shortDeadline = true
	}
	f.mu.Unlock()
	child := "child-" + req.Task
	req.Emit(assistant.SubagentEvent{RunID: child, Agent: req.Agent, Kind: assistant.SubagentStarted})
	req.Emit(assistant.SubagentEvent{RunID: child, Agent: req.Agent, Kind: assistant.SubagentToolCall, Tool: models.ToolInternetSearch, Detail: req.Task})
	req.Emit(assistant.SubagentEvent{RunID: child, Agent: req.Agent, Kind: assistant.SubagentDone})
	return assistant.SubRunResult{RunID: child, Model: "cloud-m", Status: assistant.SubRunDone, Summary: "Answer for " + req.Task + " https://example.com/" + req.Task,
		DurationMs: 1200, PromptTokens: 900, CompletionTokens: 120}
}

// A research chat hands two questions to helpers in one delegate call. The parent gets both summaries; the saved
// conversation holds only the parent's own messages and steps; the helpers' records ride on the turn's run record;
// their progress reaches the stream as nested sub-agent events; and no per-tool timeout cuts the helpers short.
func TestConversationService_DelegatesToSubAgents(t *testing.T) {
	runner := &fakeSubRunner{}
	deps := newMockConvDeps()
	deps.subRunner = runner
	pm := newTestPersistence(t)
	svc := NewConversationService(deps, pm)

	var requests []proxy.ChatRequest
	client := &assistanttest.MockClient{StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		requests = append(requests, req)
		ch := make(chan *proxy.ChatResponse, 1)
		if len(requests) == 1 {
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
				ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: models.ToolDelegate,
					Arguments: `{"tasks":[{"agent":"web-researcher","task":"A","context":"last month"},{"agent":"web-researcher","task":"B"}]}`},
			}}}}}}
		} else {
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "A and B compared: both are fine."}}}}
		}
		close(ch)
		return ch, nil
	}}
	events := &mockEventPublisher{}
	ctx := models.WithProfileRequest(context.Background(), "research")
	result, err := svc.Execute(ctx, ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv-d", Message: "compare A and B", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolInternetSearch}}}}, Client: client, Engine: &assistanttest.MockEngine{Result: "ok"}, Events: events, Recorder: nil})
	if err != nil || !strings.Contains(result.Reply, "both are fine") {
		t.Fatalf("Execute = %q, %v", result.Reply, err)
	}
	offered := false
	for _, tool := range requests[0].Tools {
		offered = offered || tool.Function.Name == models.ToolDelegate
	}
	if !offered {
		t.Errorf("a research chat offers %s", models.ToolDelegate)
	}
	if len(runner.requests) != 2 || runner.shortDeadline {
		t.Fatalf("runner got %d tasks (short deadline: %v), want 2 without the per-tool timeout", len(runner.requests), runner.shortDeadline)
	}
	for _, req := range runner.requests {
		if req.Agent != "web-researcher" || req.WorkspaceID != "ws-1" {
			t.Errorf("task %+v", req)
		}
	}
	last := requests[len(requests)-1].Messages
	if toolMsg := last[len(last)-1].Content; !strings.Contains(toolMsg, "Answer for A") || !strings.Contains(toolMsg, "Answer for B") {
		t.Errorf("the parent's next request carries both summaries, got %q", toolMsg)
	}

	saved, err := pm.ReadSession("ws-1", "conv-d")
	if err != nil || saved == nil {
		t.Fatalf("read session: %v", err)
	}
	var roles []string
	for _, m := range saved.History {
		roles = append(roles, string(m.Role))
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,assistant" {
		t.Errorf("saved roles = %s, want only the parent's turn", got)
	}
	run := saved.History[1].Run
	if run == nil || len(run.Subagents) != 2 || run.Subagents[0].Status != string(assistant.SubRunDone) || run.Subagents[0].PromptTokens != 900 {
		t.Errorf("turn record = %+v, want two done sub-agent records", run)
	}

	nested, steps := 0, 0
	for _, ev := range events.Published {
		if ev.Type == assistant.EventSubagent {
			nested++
		}
		if ev.Type == assistant.EventLifecycle && strings.Contains(fmt.Sprint(ev.Payload), "Step ") {
			steps++
		}
	}
	if nested != 6 || steps != 1 {
		t.Errorf("events: %d sub-agent, %d parent steps; want 6 nested and 1 step", nested, steps)
	}
}

// A message that asks for helpers reaches the model with a note saying so, on the model's copy only: what is stored and
// shown is what the user typed. Nothing is added when the run cannot delegate.
func TestConversationService_HelpersNote(t *testing.T) {
	cases := []struct {
		name     string
		runner   assistant.SubRunner
		profile  string
		asked    bool
		wantNote bool
	}{
		{"asked, a research chat with helpers on", &fakeSubRunner{}, "research", true, true},
		{"not asked", &fakeSubRunner{}, "research", false, false},
		{"asked, but the profile names no helpers", &fakeSubRunner{}, "general", true, false},
		{"asked, but helpers are off", &offSubRunner{}, "research", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newMockConvDeps()
			deps.subRunner = tc.runner
			pm := newTestPersistence(t)
			svc := NewConversationService(deps, pm)
			var lastUser string
			client := &assistanttest.MockClient{StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
				for _, m := range req.Messages {
					if m.Role == proxy.UserRole {
						lastUser = m.Content
					}
				}
				ch := make(chan *proxy.ChatResponse, 1)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Here is a helpful response"}}}}
				close(ch)
				return ch, nil
			}}
			ctx := models.WithHelpersRequest(models.WithProfileRequest(context.Background(), tc.profile), tc.asked)
			if _, err := svc.Execute(ctx, ExecuteRequest{WorkspaceID: "ws-1", ConversationID: "conv-h", Message: "compare A and B", ContextVersion: "v1", Timezone: "UTC", ExcludeTools: nil, Log: logging.NewNopLogger(), Provider: &assistanttest.MockProvider{}, Client: client, Engine: &assistanttest.MockEngine{Result: "ok"}, Events: &mockEventPublisher{}, Recorder: nil}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := strings.Contains(lastUser, prompts.HelpersNote); got != tc.wantNote {
				t.Errorf("the model's message has the helpers note = %v, want %v:\n%s", got, tc.wantNote, lastUser)
			}
			saved, err := pm.ReadSession("ws-1", "conv-h")
			if err != nil || saved == nil {
				t.Fatalf("read session: %v", err)
			}
			if strings.Contains(saved.History[1].Content, prompts.HelpersNote) || saved.History[1].Content != "compare A and B" {
				t.Errorf("the stored message is not what the user typed: %q", saved.History[1].Content)
			}
		})
	}
}

func TestFinishTurnRun_RecordsContextOverflows(t *testing.T) {
	tracker := &usage.Tracker{}
	tracker.AddContextOverflow()
	run := &models.TurnRun{StartedAt: time.Now()}
	finishTurnRun(run, tracker, nil)
	if run.ContextOverflows != 1 {
		t.Errorf("ContextOverflows = %d, want 1", run.ContextOverflows)
	}
}
