package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type mockConvDeps struct {
	modelName  string
	guardrail  *guardrails.GuardrailEngine
	store      *GuardrailDecisionStore
	events     EventPublisher
	log        logging.Logger
}

func (m *mockConvDeps) SelectModels() (string, string) { return m.modelName, "" }
func (m *mockConvDeps) ModelConfig(string) (models.ModelConfig, bool) {
	return models.ModelConfig{MaxSteps: 2, MaxTokens: 100}, true
}
func (m *mockConvDeps) ProcessLogger(string) logging.Logger { return m.log }
func (m *mockConvDeps) GuardrailEngine() *guardrails.GuardrailEngine { return m.guardrail }
func (m *mockConvDeps) GuardrailDecisionStore() *GuardrailDecisionStore { return m.store }
func (m *mockConvDeps) Orchestrator() *orchestrator.Orchestrator { return nil }
func (m *mockConvDeps) MemoryStore() *memory.Store { return nil }
func (m *mockConvDeps) Events() EventPublisher { return m.events }
func (m *mockConvDeps) RunLoggingEnabled() bool { return false }
func (m *mockConvDeps) RootDir() string { return "" }

type mockEventPublisher struct {
	Published []AgentEvent
	Cleared   []string
}

func (m *mockEventPublisher) Publish(workspaceID string, event AgentEvent) {
	m.Published = append(m.Published, event)
}
func (m *mockEventPublisher) Clear(workspaceID string, channel EventChannel) {
	m.Cleared = append(m.Cleared, workspaceID)
}

// latestConversationID returns the conversation_id carried by the most recent
// lifecycle event, or "" when none was published. Used by tests to locate the
// persisted session for a run whose ID is generated server-side.
func (m *mockEventPublisher) latestConversationID() string {
	for i := len(m.Published) - 1; i >= 0; i-- {
		ev := m.Published[i]
		if ev.Type != EventLifecycle {
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
		store:     NewGuardrailDecisionStore(),
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

	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"ws-1", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
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

	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
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

	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, context.Canceled
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"ws-1", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
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
		collected []AgentEvent
		want      string
	}{
		{
			name:      "no events",
			collected: nil,
			want:      "",
		},
		{
			name: "empty upstream has no failure",
			collected: []AgentEvent{
				{Type: EventUpstream, Payload: UpstreamEventPayload{Event: "retry", Attempt: 1, MaxAttempts: 3}},
			},
			want: "",
		},
		{
			name: "upstream transport error surfaces",
			collected: []AgentEvent{
				{Type: EventUpstream, Payload: UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: `Post "http://llama:8082": connection refused`, ErrClass: "connection-refused"}},
			},
			want: `Post "http://llama:8082": connection refused`,
		},
		{
			name: "terminal error wins over earlier upstream",
			collected: []AgentEvent{
				{Type: EventUpstream, Payload: UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: "retry text"}},
				{Type: EventError, Payload: map[string]string{"error": "final failure"}},
			},
			want: "final failure",
		},
		{
			name: "most recent upstream wins over earlier ones",
			collected: []AgentEvent{
				{Type: EventUpstream, Payload: UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3, Error: "first"}},
				{Type: EventUpstream, Payload: UpstreamEventPayload{Event: "retry", Reason: "transport", Attempt: 2, MaxAttempts: 3, Error: "second"}},
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

	collected := []AgentEvent{
		{Type: EventUpstream, Payload: UpstreamEventPayload{
			Event: "retry", Reason: "transport", Attempt: 1, MaxAttempts: 3,
			Error: `Post "http://192.168.50.60:8082/v1/chat/completions": dial tcp: connect: connection refused`,
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

	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("no stream")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, errors.New("model unavailable")
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	_, err := svc.Execute(
		context.Background(),
		"ws-1", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
	if err == nil {
		t.Fatal("expected error from agent execution")
	}

	// A terminal mid-run upstream failure must surface to the UI: an error
	// event (the frontend renders an error segment + clears loading) and a
	// session_completed lifecycle so the sidebar stops showing it as running.
	var gotErr bool
	var gotCompleted bool
	for _, ev := range pub.Published {
		if ev.Type == EventError {
			gotErr = true
			if _, ok := ev.Payload.(map[string]string); !ok {
				t.Errorf("error event payload should be map[string]string, got %T", ev.Payload)
			}
		}
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseSessionCompleted {
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
	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"ws-1", sessionID, "second message", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
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

	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"ws-1", "", "hello", "v1", "UTC",
		[]string{"tool-a", "tool-b"}, log, provider, client, engine, pub, nil,
	)
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

	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &mockEventPublisher{}
	log := logging.NewNopLogger()

	result, err := svc.Execute(
		context.Background(),
		"ws-persist", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	)
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
		obs(AgentEvent{
			Type: EventToolResult,
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

// orderedEventPublisher records the sequence of Clear and Publish operations so
// a test can assert ordering between the recent-buffer clear (setupRun) and the
// session_started lifecycle publish. It mirrors the EventBus semantics (Clear
// wipes the `recent` replay buffer) without importing automation (which would
// create an import cycle from the assistant test package).
type orderedEventPublisher struct {
	ops       []string
	published []AgentEvent
}

func (o *orderedEventPublisher) Publish(workspaceID string, event AgentEvent) {
	o.published = append(o.published, event)
	if event.Type == EventLifecycle {
		if p, ok := event.Payload.(map[string]any); ok && p["phase"] == PhaseSessionStarted {
			o.ops = append(o.ops, "publish:session_started")
			return
		}
	}
	o.ops = append(o.ops, "publish")
}

func (o *orderedEventPublisher) Clear(workspaceID string, channel EventChannel) {
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

	client := &MockClient{
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
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{Result: "ok"}
	pub := &orderedEventPublisher{}
	log := logging.NewNopLogger()

	if _, err := svc.Execute(
		context.Background(),
		"ws-1", "", "hello", "v1", "UTC",
		nil, log, provider, client, engine, pub, nil,
	); err != nil {
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
