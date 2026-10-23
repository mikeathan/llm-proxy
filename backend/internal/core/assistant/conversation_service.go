package assistant

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"llm-proxy/internal/core/assistant/failures"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/memorycapture"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// ConversationDeps provides the external dependencies for ConversationService.
// Implemented by the HTTP handler layer.
type ConversationDeps interface {
	RunOptionDeps
	SelectModels() (string, string)
	// Profiles is the prompt-profile catalog a run's behaviour is resolved from.
	Profiles() *profile.Catalog
	// EffectiveToolCallFormat resolves the model's tool_call_format, probing
	// local endpoints for native tool support when unset (cached).
	EffectiveToolCallFormat(ctx context.Context, modelName string) string
	ProcessLogger(workspaceID string) logging.Logger
	Events() EventPublisher
	RunLoggingEnabled() bool
}

type conversationService struct {
	deps        ConversationDeps
	persistence *persistence.WorkspaceManager
	extractor   *memorycapture.Extractor
}

func NewConversationService(deps ConversationDeps, persistence *persistence.WorkspaceManager) ConversationService {
	return &conversationService{deps: deps, persistence: persistence, extractor: memorycapture.NewExtractor(tools.ContainsSecret)}
}

func (s *conversationService) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	workspaceID, conversationID, message, timezone := req.WorkspaceID, req.ConversationID, req.Message, req.Timezone
	log, events := req.Log, req.Events
	if workspaceID == "" {
		workspaceID = "default"
	}

	// 1. Resolve or create session
	session, err := s.resolveSession(workspaceID, conversationID, req.ContextVersion, timezone, log)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("resolve session: %w", err)
	}

	// The conversation's zone (fixed when it began, as its prompt shows) is what the
	// agent's tools read a local time in.
	chatZone := cmp.Or(session.Timezone, timezone)
	ctx = models.WithChatTimezone(ctx, chatZone)

	// 2. Model config
	modelName, useNativeTools := s.resolveModelConfig(ctx, workspaceID, conversationID, message)

	// 3. Build or update history under the run's prompt profile
	runProfile := s.resolveProfile(ctx, workspaceID, session)
	s.initSession(session, workspaceID, message, conversationSystemMessage(s.persistence, session, chatZone, runProfile, useNativeTools), log)

	// The turn's run record rides on its user message: set now so mid-run
	// checkpoints carry the model and start, finished when the agent returns.
	run := startTurnRun(session.History, modelName)
	s.captureMemories(ctx, run, workspaceID, session.ID, message, runProfile, log)

	// 4. Run setup
	execCtx, clean := s.setupRun(ctx, session.ID, workspaceID, events)
	defer clean()

	// Publish session_started AFTER setupRun clears the recent buffer so the
	// event is retained in `recent` for replay. A refreshed / reopened tab
	// reconstructs the running turn from the replay, anchored by this
	// session_started and the user-message snippet it carries — without it the
	// frontend has no user anchor and cannot render the reconstructed turn.
	PublishSessionLifecycle(events, workspaceID, session.ID, message, PhaseSessionStarted)

	// Snapshot the session after the user message is appended — used as the
	// base for mid-execution checkpoint rebuilding.
	baseHistory := make([]proxy.Message, len(session.History))
	copy(baseHistory, session.History)

	// 5. Build agent options
	observer, collected := s.buildObserver(baseHistory, session, workspaceID, events, req.Recorder, log)

	agent := NewAgent(req.Client, req.Provider, req.Engine, s.agentOptions(chatAgentRun{
		modelName: modelName, workspaceID: workspaceID, sessionID: session.ID, log: log,
		observer: observer, excludeTools: req.ExcludeTools, profile: runProfile,
	}))

	// 6. Execute agent
	llmHistory := withLocalTime(FilterCancelledTurns(session.History, session.CancelledIndices), time.Now(), chatZone)
	if s.helpersAsked(ctx, runProfile) {
		llmHistory = withHelpersNote(llmHistory)
	}
	reply, updatedHistory, agErr := agent.Execute(execCtx, llmHistory)
	finishTurnRun(run, usage.FromContext(execCtx), agent.ToolWarnings())

	// 8. Handle result
	if agErr != nil {
		if errors.Is(agErr, context.Canceled) {
			return s.handleCancelResult(session, workspaceID, reply, updatedHistory, llmHistory, events, collected(), log, agErr), nil
		}
		log.Error("assistant execution failed", "error", agErr)
		return s.handleErrorResult(session, workspaceID, reply, updatedHistory, llmHistory, events, collected(), log, agErr)
	}

	// 9. Persist final history
	return s.handleSuccessResult(session, workspaceID, reply, updatedHistory, llmHistory, events, collected(), log), nil
}

// startTurnRun attaches a run record to the turn's user message — the last
// message after initSession — and returns it (nil when there is none). Queue
// wait is not counted: the run lane admits the chat before Execute.
func startTurnRun(history []proxy.Message, modelName string) *models.TurnRun {
	if len(history) == 0 || history[len(history)-1].Role != proxy.UserRole {
		return nil
	}
	run := &models.TurnRun{Model: modelName, StartedAt: time.Now()}
	history[len(history)-1].Run = run
	return run
}

// finishTurnRun stamps how long the turn ran, the tokens the provider
// reported and the run's tool warnings; a nil record is a no-op.
func finishTurnRun(run *models.TurnRun, tracker *usage.Tracker, warnings []string) {
	if run == nil {
		return
	}
	run.DurationMs = time.Since(run.StartedAt).Milliseconds()
	run.Warnings = warnings
	if tracker != nil {
		run.Subagents = tracker.Subagents()
		run.ContextOverflows = tracker.ContextOverflows()
		run.PromptTokens, run.CompletionTokens = tracker.ReportedUsage()
		run.CachedTokens = tracker.ReportedCached()
	}
}

// NormalizeConversationID returns the given conversation ID, or generates a
// fresh "conv_<timestamp>" ID when it is empty. It is the single source of
// truth for new-conversation ID generation, used both when resolving a session
// and when creating the per-run recording directory so the two agree on the
// same ID for a brand-new conversation.
func NormalizeConversationID(conversationID string) string {
	if conversationID == "" {
		return "conv_" + time.Now().Format("20060102150405")
	}
	return conversationID
}

func (s *conversationService) resolveSession(workspaceID, conversationID, contextVersion, timezone string, log logging.Logger) (*models.AssistantSession, error) {
	session, sErr := s.persistence.ReadSession(workspaceID, conversationID)
	if sErr != nil {
		log.Error("failed to load session", "error", sErr)
		return nil, fmt.Errorf("persistence error: %w", sErr)
	}

	if session == nil {
		conversationID = NormalizeConversationID(conversationID)
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

func (s *conversationService) resolveModelConfig(ctx context.Context, workspaceID, conversationID, message string) (modelName string, useNativeTools bool) {
	procLog := s.deps.ProcessLogger(workspaceID)
	procLog.Info("Assistant request started", "conversation", conversationID, "message", message)

	// The operator may pick the model for this one message; otherwise the primary.
	if modelName = models.GetModelOverride(ctx); modelName == "" {
		modelName, _ = s.deps.SelectModels()
	}
	// Effective format (explicit override / cloud native default / cached
	// local capability probe) — the same resolution automations use.
	useNativeTools = s.deps.EffectiveToolCallFormat(ctx, modelName) == "native"
	return
}

// resolveProfile picks the run's prompt profile: the one this message asked for (which the conversation then keeps),
// else the conversation's, else the workspace's, else the default for chats.
func (s *conversationService) resolveProfile(ctx context.Context, workspaceID string, session *models.AssistantSession) profile.Profile {
	if requested := models.GetProfileRequest(ctx); requested != "" {
		session.Profile = requested
	}
	var workspaceDefault string
	if cfg, err := s.persistence.ReadConfig(workspaceID); err == nil {
		workspaceDefault = cfg.AssistantProfile
	}
	return s.deps.Profiles().Resolve(profile.KindAssistant, session.Profile, workspaceDefault)
}

// initSession puts the turn's user message into the session, under the current system message, and saves it.
func (s *conversationService) initSession(session *models.AssistantSession, workspaceID, message, system string, log logging.Logger) {
	session.History = append(session.History, proxy.Message{Role: proxy.UserRole, Content: message})
	session.History = withSystemMessage(session.History, system)
	session.History = TruncateHistory(session.History, MaxHistoryChars)

	// Persist early so the session appears in the conversation sidebar
	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Warn("failed to persist session on user message", "error", pErr)
	}
}

func (s *conversationService) setupRun(ctx context.Context, sessionID, workspaceID string, events EventPublisher) (execCtx context.Context, clean func()) {
	// Reuse a run ID already threaded into the context (the HTTP handler sets
	// it so the recorder's SetDirForRun/CloseRun keys match the agent's
	// recording file). A mismatched ID orphaned the recording file + fd + sync
	// goroutine forever (the recorder's CloseRun could never find the state
	// written under the service-generated ID).
	runID := models.GetRunID(ctx)
	if runID == "" {
		runID = models.NewRunID()
	}
	execCtx = models.WithTaskName(ctx, sessionID)
	execCtx = models.WithRunID(execCtx, runID)
	execCtx = usage.WithTracker(execCtx)

	events.Clear(workspaceID, ChannelAssistant)
	// Clear stale events from previous runs so new SSE connection doesn't replay old events
	clean = func() { events.Clear(workspaceID, ChannelAssistant) }
	return
}

func (s *conversationService) buildObserver(baseHistory []proxy.Message, session *models.AssistantSession, workspaceID string, events EventPublisher, recorder EventRecorder, log logging.Logger) (Observer, func() []AgentEvent) {
	var collectedEvents []AgentEvent
	sessionStep := 0

	obs := func(ev AgentEvent) {
		// Keep the newest snapshot of a stream only: reasoning and tool_stream
		// carry the text so far, and buildPartialHistory rescans this log on every
		// tool result, so keeping each one cost ~11 MB and a quadratic rescan on
		// a four-minute run.
		if n := len(collectedEvents); n > 0 && SupersedesSnapshot(collectedEvents[n-1], ev) {
			collectedEvents = collectedEvents[:n-1]
		}
		collectedEvents = append(collectedEvents, ev)
		events.Publish(workspaceID, ev)
		if recorder != nil {
			if recErr := recorder.Write(ev); recErr != nil {
				log.Warn("failed to write event to recording", "error", recErr)
			}
		}
		// Checkpoint the session on every completed tool cycle / final message so
		// a page refresh (or a click on the running session from the sidebar)
		// returns the latest committed tool calls and results — the documented
		// contract (session-source-backend-driven.md) is unconditional, no
		// throttling. The copy is shallow: the original session.History is never
		// mutated mid-run, so the final success/cancel paths append cleanly.
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

// chatAgentRun is what one chat turn's agent is built from.
type chatAgentRun struct {
	modelName, workspaceID, sessionID string
	log                               logging.Logger
	observer                          Observer
	excludeTools                      []string
	profile                           profile.Profile
}

// agentOptions resolves the options a chat turn's agent runs with; the workspace's memory setting is the turn's own
// choice, above the profile's.
func (s *conversationService) agentOptions(r chatAgentRun) AgentOptions {
	return ResolveRunOptions(s.deps, RunSpec{
		Kind: profile.KindAssistant, WorkspaceID: r.workspaceID, ConversationID: r.sessionID, Model: r.modelName,
		Profile: r.profile, Logger: r.log, Observer: r.observer,
		Overrides: RunOverrides{ExcludedTools: r.excludeTools, Memory: s.workspaceMemory(r.workspaceID)},
	})
}

// workspaceMemory is the workspace's hot-memory setting for its chats (inherit when it has none).
func (s *conversationService) workspaceMemory(workspaceID string) models.MemoryMode {
	if cfg, err := s.persistence.ReadConfig(workspaceID); err == nil {
		return cfg.AssistantMemory
	}
	return models.MemoryModeInherit
}

func (s *conversationService) handleCancelResult(session *models.AssistantSession, workspaceID, reply string, updatedHistory, llmHistory []proxy.Message, events EventPublisher, collected []AgentEvent, log logging.Logger, cancelErr error) ExecuteResult {
	log.Info("assistant execution canceled by user", "error", cancelErr)

	if len(updatedHistory) >= len(llmHistory) {
		cancelNewMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, cancelNewMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory (cancel)", "updated", len(updatedHistory), "llm", len(llmHistory))
	}

	// Preserve the failure that prompted the cancel (e.g. upstream connection
	// errors) so a reloaded session shows why the run stopped instead of a bare
	// "Response interrupted" marker. Mirrors handleErrorResult's error marker;
	// best-effort no-op when the run saw no failures.
	if errText := lastRunFailureText(collected); errText != "" {
		session.History = append(session.History, proxy.Message{
			Role:    proxy.AssistantRole,
			Error:   errText,
			Content: "",
		})
	}

	_, canceledUserIdx := ComputeCancelIndices(session.History)
	if canceledUserIdx >= 0 {
		session.CancelledIndices = append(session.CancelledIndices, canceledUserIdx)
	}

	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session after cancel", "error", pErr)
	}
	endAssistantRun(events, workspaceID, session.ID)

	return ExecuteResult{
		Reply:          reply,
		ConversationID: session.ID,
		WorkspaceID:    session.WorkspaceID,
		Events:         FormatCollectedEvents(collected),
		Canceled:       true,
	}
}

// lastRunFailureText returns the most recent failure text from the collected
// run events, or "" when the run saw no failures. It checks terminal error
// events first, then upstream transport/status retry notices (which carry the
// provider error text). Used by the cancel path to preserve why the run was
// interrupted so a reloaded session renders the error instead of a bare
// "Response interrupted" banner.
func lastRunFailureText(collected []AgentEvent) string {
	for i := len(collected) - 1; i >= 0; i-- {
		ev := collected[i]
		switch ev.Type {
		case EventError:
			if m, ok := ev.Payload.(map[string]string); ok && m["error"] != "" {
				return m["error"]
			}
		case EventUpstream:
			if p, ok := ev.Payload.(UpstreamEventPayload); ok && p.Error != "" {
				return p.Error
			}
		}
	}
	return ""
}

func (s *conversationService) handleSuccessResult(session *models.AssistantSession, workspaceID, reply string, updatedHistory, llmHistory []proxy.Message, events EventPublisher, collected []AgentEvent, log logging.Logger) ExecuteResult {
	if len(updatedHistory) >= len(llmHistory) {
		newMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, newMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory", "updated", len(updatedHistory), "llm", len(llmHistory))
	}
	session.History = TruncateHistory(session.History, MaxPersistedHistoryChars)

	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
	}
	endAssistantRun(events, workspaceID, session.ID)

	return ExecuteResult{
		Reply:          reply,
		ConversationID: session.ID,
		WorkspaceID:    session.WorkspaceID,
		Events:         FormatCollectedEvents(collected),
	}
}

// handleErrorResult persists a terminal (non-cancel) run failure and surfaces it
// to the UI. Unlike the success path, `updatedHistory` may be empty or partial
// (an upstream failure can occur before any assistant output), so an explicit
// assistant-role error message is appended to guarantee the failure survives a
// reload instead of leaving a session that shows only the user prompt. The
// error event + session_completed lifecycle are still published so a live
// client sees the failure and the sidebar stops marking the session running.
func (s *conversationService) handleErrorResult(session *models.AssistantSession, workspaceID, reply string, updatedHistory, llmHistory []proxy.Message, events EventPublisher, collected []AgentEvent, log logging.Logger, runErr error) (ExecuteResult, error) {
	// Persist any partial assistant output the agent produced before failing.
	if len(updatedHistory) >= len(llmHistory) {
		newMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, newMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory (error)", "updated", len(updatedHistory), "llm", len(llmHistory))
	}
	// Always append an explicit error marker so the failure is visible after a
	// reload even when the run produced no assistant output at all. The text is
	// the classified summary + hint — a raw upstream error body would be an
	// opaque JSON dump in the chat (the full error is in the logs).
	fi := failures.ClassifyRunFailure(runErr)
	errText := fi.Error
	if fi.Hint != "" {
		errText = fi.Error + "\n\n" + fi.Hint
	}
	session.History = append(session.History, proxy.Message{
		Role:    proxy.AssistantRole,
		Error:   errText,
		Content: "",
	})
	session.History = TruncateHistory(session.History, MaxPersistedHistoryChars)
	if pErr := s.persistence.WriteSession(workspaceID, session); pErr != nil {
		log.Error("failed to save session after error", "error", pErr)
	}

	// Surface the terminal mid-run failure to the UI: publish an error event
	// (the frontend renders an error segment and clears loading) and a
	// session_completed lifecycle so the sidebar stops showing the session as
	// running. Previously this failure only reached the logs, leaving the
	// bubble stuck on "thinking". The payload carries the classified summary +
	// optional hint (bounded, human-actionable) instead of the raw error.
	payload := map[string]string{"error": fi.Error}
	if fi.Hint != "" {
		payload["hint"] = fi.Hint
	}
	events.Publish(workspaceID, AgentEvent{
		ID:             fmt.Sprintf("sse_err_%d", time.Now().UnixNano()),
		Type:           EventError,
		Channel:        ChannelAssistant,
		ConversationID: session.ID,
		Payload:        payload,
		Timestamp:      time.Now(),
	})
	endAssistantRun(events, workspaceID, session.ID)

	return ExecuteResult{
		ConversationID: session.ID,
		WorkspaceID:    session.WorkspaceID,
		Events:         FormatCollectedEvents(collected),
	}, fmt.Errorf("assistant execution failed: %w", runErr)
}

// endAssistantRun seals a finished assistant run: it publishes the completed
// lifecycle (so the sidebar stops marking the session running) AND clears the
// channel's recent-event buffer. `recent` only needs to survive mid-run
// refreshes (a reloading client rebuilds the live turn from it); once the run
// is finished its history lives on disk. Clearing at the end — instead of only
// at the next run's setupRun — stops a finished run's events (stale "model is
// starting" notices, assistant messages) from replaying into a fresh/empty
// conversation view after a reconnect or new send.
func endAssistantRun(events EventPublisher, workspaceID, sessionID string) {
	PublishSessionLifecycle(events, workspaceID, sessionID, "", PhaseSessionCompleted)
	events.Clear(workspaceID, ChannelAssistant)
}

// helpersAsked reports whether this message asked for helpers and the run can use them: its profile names helpers and
// the sub-runner is set up. Otherwise the request is ignored, never an error.
func (s *conversationService) helpersAsked(ctx context.Context, p profile.Profile) bool {
	if !models.HelpersRequested(ctx) || len(p.Delegates) == 0 {
		return false
	}
	runner := s.deps.SubRunner()
	return runner != nil && runner.Enabled()
}
