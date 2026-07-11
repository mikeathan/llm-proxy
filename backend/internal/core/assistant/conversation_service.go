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

func (s *conversationService) Execute(ctx context.Context, workspaceID, conversationID, message, contextVersion, timezone string, excludeTools []string, log logging.Logger, provider ToolProvider, client proxy.Client, engine Engine, events EventPublisher, recorder EventRecorder) (any, error) {
	if workspaceID == "" {
		workspaceID = "default"
	}

	// 1. Resolve or create session
	session, err := s.resolveSession(workspaceID, conversationID, contextVersion, timezone, log)
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}

	// 2. Model config
	procLog := s.deps.ProcessLogger(workspaceID)
	procLog.Info("Assistant request started", "conversation", conversationID, "message", message)

	modelName, _ := s.deps.SelectModels()
	useNativeTools := false
	if cfg, ok := s.deps.ModelConfig(modelName); ok {
		useNativeTools = cfg.ToolCallFormat == "native"
	}

	// 3. Build or update history
	if len(session.History) == 0 {
		initial, bErr := BuildInitialHistory(s.persistence, workspaceID, conversationID, message, contextVersion, timezone, useNativeTools)
		if bErr != nil {
			return nil, fmt.Errorf("build history: %w", bErr)
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
	PublishSessionLifecycle(events, workspaceID, session.ID, message, PhaseSessionStarted)

	// 4. Run setup
	runID := GenerateRunID()
	execCtx := models.WithTaskName(ctx, session.ID)
	execCtx = models.WithRunID(execCtx, runID)
	execCtx = WithUsageTracker(execCtx)

	// Clear stale events from previous runs so new SSE connection doesn't replay old events
	events.Clear(workspaceID)
	defer events.Clear(workspaceID)

	// 5. Build agent options
	// Snapshot the session after the user message is appended — used as the
	// base for mid-execution checkpoint rebuilding.
	baseHistory := make([]proxy.Message, len(session.History))
	copy(baseHistory, session.History)

	var collectedEvents []AgentEvent
	sessionStep := 0
	publishObs := func(ev AgentEvent) {
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

	if len(excludeTools) > 0 {
		provider = NewFilteredToolProvider(provider, excludeTools)
	}

	builder := NewAgentBuilder(s).
		WithLogger(log).
		WithGuardrails().
		WithWorkspaceID(workspaceID).
		WithModelName(modelName).
		WithHotMemory(true).
		WithObserver(publishObs).
		WithGuardrailDecisionHandler(NewGuardrailDecisionCallback(s.deps.GuardrailDecisionStore(), publishObs)).
		WithOrchestrator().
		WithModelConfig(ctx, modelName, provider, client)

	agent := builder.Build(client, provider, engine)

	// 6. Execute agent
	llmHistory := FilterCancelledTurns(session.History, session.CancelledIndices)
	reply, updatedHistory, agErr := agent.Execute(execCtx, llmHistory)

	// 8. Handle result
	if agErr != nil {
		if errors.Is(agErr, context.Canceled) {
			log.Info("assistant execution canceled by user", "error", agErr)
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
			return map[string]any{
				"reply":           reply,
				"conversation_id": session.ID,
				"workspace_id":    session.WorkspaceID,
				"events":          FormatCollectedEvents(collectedEvents),
				"canceled":        true,
			}, nil
		}
		log.Error("assistant execution failed", "error", agErr)
		return nil, fmt.Errorf("assistant execution failed: %w", agErr)
	}

	// 9. Persist final history
	if len(updatedHistory) >= len(llmHistory) {
		newMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, newMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory", "updated", len(updatedHistory), "llm", len(llmHistory))
	}
	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
	}
	PublishSessionLifecycle(events, workspaceID, session.ID, "", PhaseSessionCompleted)

	return map[string]any{
		"reply":           reply,
		"conversation_id": session.ID,
		"workspace_id":    session.WorkspaceID,
		"events":          FormatCollectedEvents(collectedEvents),
	}, nil
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
