package assistant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// ConversationDeps provides the external dependencies for ConversationService.
// Implemented by the HTTP handler layer.
type ConversationDeps interface {
	SelectModels() (string, string)
	ModelConfig(modelName string) (models.ModelConfig, bool)
	ProcessLogger(workspaceID string) logging.Logger
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *GuardrailDecisionStore
	Orchestrator() *orchestrator.Orchestrator
	MemoryStore() *memory.Store
	Events() EventPublisher
	RunLoggingEnabled() bool
	RootDir() string
}

type conversationService struct {
	deps        ConversationDeps
	persistence *persistence.WorkspaceManager
}

func NewConversationService(deps ConversationDeps, persistence *persistence.WorkspaceManager) ConversationService {
	return &conversationService{deps: deps, persistence: persistence}
}

func (s *conversationService) Execute(ctx context.Context, workspaceID, conversationID, message, contextVersion, timezone string, excludeTools []string, log logging.Logger, provider ToolProvider, client proxy.Client, engine Engine, events EventPublisher, recorder EventRecorder) (ExecuteResult, error) {
	if workspaceID == "" {
		workspaceID = "default"
	}

	// 1. Resolve or create session
	session, err := s.resolveSession(workspaceID, conversationID, contextVersion, timezone, log)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("resolve session: %w", err)
	}

	// 2. Model config
	modelName, useNativeTools := s.resolveModelConfig(workspaceID, conversationID, message)

	// 3. Build or update history
	if err := s.initSession(session, workspaceID, conversationID, message, contextVersion, timezone, useNativeTools, log); err != nil {
		return ExecuteResult{}, fmt.Errorf("init session: %w", err)
	}

	PublishSessionLifecycle(events, workspaceID, session.ID, message, PhaseSessionStarted)

	// 4. Run setup
	execCtx, clean := s.setupRun(ctx, session.ID, workspaceID, events)
	defer clean()

	// Snapshot the session after the user message is appended — used as the
	// base for mid-execution checkpoint rebuilding.
	baseHistory := make([]proxy.Message, len(session.History))
	copy(baseHistory, session.History)

	// 5. Build agent options
	observer, collected := s.buildObserver(baseHistory, session, workspaceID, events, recorder, log)

	if len(excludeTools) > 0 {
		provider = NewFilteredToolProvider(provider, excludeTools)
	}

	agent := s.buildAgent(ctx, modelName, workspaceID, session.ID, log, provider, client, engine, observer)

	// 6. Execute agent
	llmHistory := FilterCancelledTurns(session.History, session.CancelledIndices)
	reply, updatedHistory, agErr := agent.Execute(execCtx, llmHistory)

	// 8. Handle result
	if agErr != nil {
		if errors.Is(agErr, context.Canceled) {
			return s.handleCancelResult(session, workspaceID, reply, updatedHistory, llmHistory, events, collected(), log, agErr), nil
		}
		log.Error("assistant execution failed", "error", agErr)
		return ExecuteResult{}, fmt.Errorf("assistant execution failed: %w", agErr)
	}

	// 9. Persist final history
	return s.handleSuccessResult(session, workspaceID, reply, updatedHistory, llmHistory, events, collected(), log), nil
}

func (s *conversationService) resolveSession(workspaceID, conversationID, contextVersion, timezone string, log logging.Logger) (*models.AssistantSession, error) {
	session, sErr := s.persistence.ReadSession(workspaceID, conversationID)
	if sErr != nil {
		log.Error("failed to load session", "error", sErr)
		return nil, fmt.Errorf("persistence error: %w", sErr)
	}

	if session == nil {
		if conversationID == "" {
			conversationID = "conv_" + time.Now().Format("20060102150405")
		}
		session = &models.AssistantSession{
			ID:             conversationID,
			WorkspaceID:    workspaceID,
			ContextVersion: contextVersion,
			Timezone:       timezone,
			History:        []proxy.Message{},
		}
	}
	return session, nil
}

func (s *conversationService) resolveModelConfig(workspaceID, conversationID, message string) (modelName string, useNativeTools bool) {
	procLog := s.deps.ProcessLogger(workspaceID)
	procLog.Info("Assistant request started", "conversation", conversationID, "message", message)

	modelName, _ = s.deps.SelectModels()
	if cfg, ok := s.deps.ModelConfig(modelName); ok {
		useNativeTools = cfg.ToolCallFormat == "native"
	}
	return
}

func (s *conversationService) initSession(session *models.AssistantSession, workspaceID, conversationID, message, contextVersion, timezone string, useNativeTools bool, log logging.Logger) error {
	if len(session.History) == 0 {
		initial, bErr := BuildInitialHistory(s.persistence, workspaceID, conversationID, message, contextVersion, timezone, useNativeTools)
		if bErr != nil {
			return fmt.Errorf("build history: %w", bErr)
		}
		session.History = initial
	} else {
		session.History = append(session.History, proxy.Message{
			Role:    proxy.UserRole,
			Content: message,
		})
	}

	session.History = TruncateHistory(session.History)

	// Persist early so the session appears in the conversation sidebar
	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Warn("failed to persist session on user message", "error", pErr)
	}
	return nil
}

func (s *conversationService) setupRun(ctx context.Context, sessionID, workspaceID string, events EventPublisher) (execCtx context.Context, clean func()) {
	runID := GenerateRunID()
	execCtx = models.WithTaskName(ctx, sessionID)
	execCtx = models.WithRunID(execCtx, runID)
	execCtx = WithUsageTracker(execCtx)

	events.Clear(workspaceID, ChannelAssistant)
	// Clear stale events from previous runs so new SSE connection doesn't replay old events
	clean = func() { events.Clear(workspaceID, ChannelAssistant) }
	return
}

func (s *conversationService) buildObserver(baseHistory []proxy.Message, session *models.AssistantSession, workspaceID string, events EventPublisher, recorder EventRecorder, log logging.Logger) (Observer, func() []AgentEvent) {
	var collectedEvents []AgentEvent
	sessionStep := 0

	obs := func(ev AgentEvent) {
		collectedEvents = append(collectedEvents, ev)
		events.Publish(workspaceID, ev)
		if recorder != nil {
			if recErr := recorder.Write(ev); recErr != nil {
				log.Warn("failed to write event to recording", "error", recErr)
			}
		}
		if ev.Type == EventToolResult || ev.Type == EventMessage {
			cp := *session
			cp.History = buildPartialHistory(baseHistory, collectedEvents)
			if err := s.persistence.WriteSession(workspaceID, &cp); err != nil {
				log.Warn("checkpoint persist failed", "error", err, "session", session.ID)
			}
		}
		if ev.Type == EventToolCall {
			sessionStep++
			PublishSessionLifecycle(events, workspaceID, session.ID, fmt.Sprintf("Step %d: %s", sessionStep, ev.Payload), PhaseSessionProgress)
		}
	}

	return obs, func() []AgentEvent { return collectedEvents }
}

func (s *conversationService) buildAgent(ctx context.Context, modelName, workspaceID, sessionID string, log logging.Logger, provider ToolProvider, client proxy.Client, engine Engine, observer Observer) *Agent {
	builder := NewAgentBuilder(s).
		WithLogger(log).
		WithGuardrails().
		WithWorkspaceID(workspaceID).
		WithModelName(modelName).
		WithChannel(ChannelAssistant).
		WithConversationID(sessionID).
		WithHotMemory(true).
		WithObserver(observer).
		WithGuardrailDecisionHandler(NewGuardrailDecisionCallback(s.deps.GuardrailDecisionStore(), observer)).
		WithOrchestrator().
		WithModelConfig(ctx, modelName, provider, client)

	return builder.Build(client, provider, engine)
}

func (s *conversationService) handleCancelResult(session *models.AssistantSession, workspaceID, reply string, updatedHistory, llmHistory []proxy.Message, events EventPublisher, collected []AgentEvent, log logging.Logger, cancelErr error) ExecuteResult {
	log.Info("assistant execution canceled by user", "error", cancelErr)

	if len(updatedHistory) >= len(llmHistory) {
		cancelNewMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, cancelNewMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory (cancel)", "updated", len(updatedHistory), "llm", len(llmHistory))
	}

	_, canceledUserIdx := ComputeCancelIndices(session.History)
	if canceledUserIdx >= 0 {
		session.CancelledIndices = append(session.CancelledIndices, canceledUserIdx)
	}

	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session after cancel", "error", pErr)
	}
	PublishSessionLifecycle(events, workspaceID, session.ID, "", PhaseSessionCompleted)

	return ExecuteResult{
		Reply:          reply,
		ConversationID: session.ID,
		WorkspaceID:    session.WorkspaceID,
		Events:         FormatCollectedEvents(collected),
		Canceled:       true,
	}
}

func (s *conversationService) handleSuccessResult(session *models.AssistantSession, workspaceID, reply string, updatedHistory, llmHistory []proxy.Message, events EventPublisher, collected []AgentEvent, log logging.Logger) ExecuteResult {
	if len(updatedHistory) >= len(llmHistory) {
		newMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, newMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory", "updated", len(updatedHistory), "llm", len(llmHistory))
	}
	session.History = TruncateHistory(session.History)

	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
	}
	PublishSessionLifecycle(events, workspaceID, session.ID, "", PhaseSessionCompleted)

	return ExecuteResult{
		Reply:          reply,
		ConversationID: session.ID,
		WorkspaceID:    session.WorkspaceID,
		Events:         FormatCollectedEvents(collected),
	}
}

// compile-time check: ServiceProvider interface is implemented by *conversationService
var _ ServiceProvider = (*conversationService)(nil)

func (s *conversationService) ModelConfig(modelName string) (models.ModelConfig, bool) {
	return s.deps.ModelConfig(modelName)
}

func (s *conversationService) GuardrailEngine() *guardrails.GuardrailEngine {
	return s.deps.GuardrailEngine()
}

func (s *conversationService) GuardrailDecisionStore() *GuardrailDecisionStore {
	return s.deps.GuardrailDecisionStore()
}

func (s *conversationService) Orchestrator() *orchestrator.Orchestrator {
	return s.deps.Orchestrator()
}

func (s *conversationService) MemoryStore() *memory.Store {
	return s.deps.MemoryStore()
}

func (s *conversationService) Events() EventPublisher {
	return s.deps.Events()
}
