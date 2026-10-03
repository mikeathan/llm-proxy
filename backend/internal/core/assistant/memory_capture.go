package assistant

import (
	"context"

	"llm-proxy/internal/core/memorycapture"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// memorySourceCapture attributes entries saved from a user's explicit request, so the Memory panel shows where they came from.
const memorySourceCapture = "capture"

// NewCaptureLibrary exposes the memory store as a memorycapture.Library for callers outside this package (the review endpoint).
func NewCaptureLibrary(store *memory.Store) memorycapture.Library {
	return captureLibrary{tools: tools.NewMemoryToolProvider(store)}
}

// ReviewTurns reduces a session to what a memory review may read: the user's and the assistant's own words. System text,
// tool calls and results, and the agent's injected control messages are dropped; they can carry content from the web or
// from files that must not become memory through a model's summary.
func ReviewTurns(history []proxy.Message) []memorycapture.Turn {
	var turns []memorycapture.Turn
	for _, m := range history {
		if (m.Role != proxy.UserRole && m.Role != proxy.AssistantRole) || len(m.ToolCalls) > 0 || isAgentControlMessage(m) {
			continue
		}
		turns = append(turns, memorycapture.Turn{Role: string(m.Role), Text: m.Content})
	}
	return turns
}

// captureLibrary adapts the memory tool's save path to memorycapture.Library, so a captured fact and a model's
// memory_update share one set of routing and dedup rules.
type captureLibrary struct {
	tools *tools.MemoryToolProvider
}

func (l captureLibrary) Save(ctx context.Context, workspaceID string, c memorycapture.Candidate) (memorycapture.Outcome, error) {
	res, err := l.tools.SaveFact(ctx, workspaceID, tools.Fact{Content: c.Content, Scope: c.Scope, Mode: c.Mode, Keep: memory.KeepPermanent, Source: memorySourceCapture})
	if err != nil {
		return 0, err
	}
	switch res.Outcome {
	case tools.SaveCreated:
		return memorycapture.OutcomeCreated, nil
	case tools.SaveUpdated:
		return memorycapture.OutcomeUpdated, nil
	default:
		return memorycapture.OutcomeDuplicate, nil
	}
}

func (l captureLibrary) Has(ctx context.Context, workspaceID, content string) bool {
	return l.tools.HasFact(ctx, workspaceID, content)
}

// captureMemories saves what the user explicitly asked to remember and records it on the turn's run record. It runs
// before the agent starts, so a standing instruction is already in that run's memory block. Only the operator's own
// chats qualify (a connector session is an outside sender) and only while assistant memory is on. It never fails the
// chat: a store error is logged and the turn goes on.
func (s *conversationService) captureMemories(ctx context.Context, run *models.TurnRun, workspaceID, conversationID, message string, log logging.Logger) {
	store := s.deps.MemoryStore()
	if run == nil || store == nil || models.SessionSource(conversationID) != models.SessionSourceManual || !s.hotMemoryEnabled(workspaceID) {
		return
	}
	res := memorycapture.Capture(ctx, s.extractor, captureLibrary{tools: tools.NewMemoryToolProvider(store)}, workspaceID, message)
	run.MemorySaved = res.Saved
	if res.Failed > 0 {
		log.Warn("explicit memory request could not be saved", "workspace", workspaceID, "failed", res.Failed)
	}
}
