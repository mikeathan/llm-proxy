package assistant

import (
	"context"
	"errors"
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
