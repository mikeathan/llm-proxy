package automation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

// Sentinel errors for shell PGID queries from the automation executor.
var (
	ErrShellPGIDNotAvailable = errors.New("shell PGID not available")
	ErrNoShellPool           = errors.New("shell pool not available")
)

// maxCapturedEventsPerRun bounds how many agent events are retained in memory
// per run for the automation result. The complete stream is written to the
// run-dir events.jsonl; only the tail is kept in RAM.
const maxCapturedEventsPerRun = 500

// TaskExecutor executes automations.
type TaskExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
	// ShellPGID returns the negated process group ID for a workspace's
	// active shell session. Returns an error when not available.
	ShellPGID(ctx context.Context, workspaceID string) (int, error)
}

type ExecuteRequest struct {
	WorkspaceID    string
	AutomationName string
	TaskFile       string
	TaskContent    string
	Strategy       ExecutionStrategy
	State          *models.AgentState
	Model          string   // Optional model override
	AllowedTools   []string // restrict tools for unattended runs
	RecordingRef   string   // Recording file ID for playback (empty = live LLM)
}

type ExecuteResponse struct {
	Output string
	Error  error
	State  *models.AgentState
}

// LLMServiceProvider is the minimal interface needed by LLMTaskExecutor.
// It avoids an import cycle by not depending on the api or nodeherder packages.
type LLMServiceProvider interface {
	ClientProvider() proxy.LLMClientProvider
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	ModelConfig(modelName string) (models.ModelConfig, bool)
	Logger() logging.Logger
	ToolProvider() assistant.ToolProvider
	Engine() assistant.Engine
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *assistant.GuardrailDecisionStore
	ProcessLogger(workspaceID string) logging.Logger
	Persistence() *persistence.WorkspaceManager
	Events() assistant.EventPublisher
	Orchestrator() *orchestrator.Orchestrator
	MemoryStore() *memory.Store
	GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error)
	RecordDir() string
	RootDir() string
	RunLoggingEnabled() bool
	SelectModels() (string, string)
}

// DefaultTaskExecutor is a placeholder that marks execution as running.
// Kept for cases where LLM execution is not required.
type DefaultTaskExecutor struct{}

func NewDefaultTaskExecutor() *DefaultTaskExecutor {
	return &DefaultTaskExecutor{}
}

func (e *DefaultTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	resp.State.SetRunning(req.AutomationName)
	resp.Output = "dispatcher placeholder output (Phase 3: wire LLM)"

	return resp, nil
}

func (e *DefaultTaskExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	return 0, ErrShellPGIDNotAvailable
}

// LLMTaskExecutor is a TaskExecutor that uses an LLM client for execution.
type LLMTaskExecutor struct {
	svc       LLMServiceProvider
	shellPool shell.ShellProvider
}

func NewLLMTaskExecutor(svc LLMServiceProvider) TaskExecutor {
	return &LLMTaskExecutor{svc: svc}
}

// SetShellPool injects the shell provider for PGID queries during force-stop.
func (e *LLMTaskExecutor) SetShellPool(pool shell.ShellProvider) {
	e.shellPool = pool
}

// ShellPGID returns the negated process group ID for a workspace's active
// shell session. Returns an error when no shell pool is configured or no
// active session exists.
func (e *LLMTaskExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	if e.shellPool == nil {
		return 0, ErrNoShellPool
	}
	pgid, ok := e.shellPool.PGID(workspaceID)
	if !ok {
		return 0, fmt.Errorf("no active shell session for workspace %s", workspaceID)
	}
	return pgid, nil
}

func (e *LLMTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	startTime := time.Now()
	resp := &ExecuteResponse{State: req.State}
	if resp.State == nil {
		resp.State = &models.AgentState{}
	}
	req.State = resp.State
	resp.State.SetRunning(req.AutomationName)

	if req.Model == "" {
		primary, _ := e.svc.SelectModels()
		req.Model = primary
	}

	procLog := e.svc.ProcessLogger(req.WorkspaceID)
	client, err := e.getLLMClient(ctx, req, resp, startTime)
	if err != nil {
		return resp, err
	}
	procLog.Info("Automation execution started", "workspace", req.WorkspaceID, "automation", req.AutomationName)

	execCtx := models.WithTaskName(ctx, req.AutomationName)
	execCtx = models.WithRunID(execCtx, generateRunID())
	execCtx = assistant.WithUsageTracker(execCtx)

	runDir, eventSink, _, runLog := e.setupRunDir(execCtx, client, req, procLog)
	procLog = runLog
	var capturedEvents []any

	toolProvider := e.svc.ToolProvider()
	if len(req.AllowedTools) > 0 {
		toolProvider = assistant.NewAllowedToolsProvider(toolProvider, req.AllowedTools)
	}
	agentOpts := e.buildAgentOptions(execCtx, client, req, procLog, &capturedEvents, eventSink)
	agent := assistant.NewAgent(client, toolProvider, e.svc.Engine(), agentOpts)

	agentsFileContent := assistant.LoadAgentsFile(e.svc.Persistence(), req.WorkspaceID)
	useNativeTools := false
	if cfg, ok := e.svc.ModelConfig(req.Model); ok {
		useNativeTools = cfg.ToolCallFormat == "native"
	}
	systemPrompt := prompts.AssembleSystemPrompt(agentsFileContent, useNativeTools)

	history := []proxy.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: e.buildPrompt(req.TaskContent, req)},
	}

	finalReply, fullHistory, agErr := agent.Execute(execCtx, history)

	if eventSink != nil {
		eventSink.Close()
	}
	if tl, ok := procLog.(*teeLogger); ok {
		tl.Close()
	}
	if rcl, ok := client.(interface{ CloseRun(string) }); ok {
		rcl.CloseRun(models.GetRunID(execCtx))
	}

	if t := assistant.GetUsageTracker(execCtx); t != nil {
		procLog.Info("Usage", "llm_calls", t.LLMCalls, "tool_calls", t.ToolCalls,
			"input_tokens", t.InputTokens, "output_tokens", t.OutputTokens)
	}

	if agErr != nil {
		return e.handleAgentError(req, resp, procLog, execCtx, runDir, capturedEvents, startTime, agErr)
	}

	return e.handleAgentSuccess(req, resp, procLog, execCtx, runDir, capturedEvents, startTime, finalReply, fullHistory)
}

// getLLMClient returns the LLM client for a live model or playback recording.
func (e *LLMTaskExecutor) getLLMClient(ctx context.Context, req ExecuteRequest, resp *ExecuteResponse, startTime time.Time) (proxy.Client, error) {
	if req.RecordingRef != "" {
		client, err := e.svc.GetPlaybackClient(ctx, req.RecordingRef)
		if err != nil {
			errStr := fmt.Sprintf("failed to load recording %s: %v", req.RecordingRef, err)
			resp.State.LastError = errStr
			resp.State.SetRunning("")
			e.recordRun(req, resp.State, "", errStr, time.Since(startTime), nil)
			return nil, fmt.Errorf("failed to load recording: %w", err)
		}
		procLog := e.svc.ProcessLogger(req.WorkspaceID)
		procLog.Info("Running automation from recording", "recording", req.RecordingRef)
		return client, nil
	}

	var client proxy.Client
	var err error
	if req.Model != "" {
		client, err = e.svc.GetClientForModel(ctx, req.Model)
	} else {
		client, err = e.svc.ClientProvider().GetClient(ctx)
	}
	if err != nil {
		errStr := fmt.Sprintf("failed to get llm client: %v", err)
		resp.State.LastError = errStr
		resp.State.SetRunning("")
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime), nil)
		return nil, fmt.Errorf("failed to get llm client: %w", err)
	}
	return client, nil
}

func (e *LLMTaskExecutor) setupRunDir(ctx context.Context, client proxy.Client, req ExecuteRequest, procLog logging.Logger) (*RunDir, *EventSink, bool, logging.Logger) {
	if !e.svc.RunLoggingEnabled() {
		return nil, nil, false, procLog
	}
	rootDir := e.svc.RootDir()
	if rootDir == "" || req.Model == "" {
		return nil, nil, false, procLog
	}
	parent := filepath.Join(rootDir, "runs")
	runDir, rErr := NewRunDir(parent, req.WorkspaceID, req.AutomationName, req.Model)
	if rErr != nil {
		procLog.Warn("failed to create run dir, continuing without per-run output", "error", rErr)
		return nil, nil, false, procLog
	}
	eventSink, esErr := NewEventSink(runDir.EventsPath())
	if esErr != nil {
		procLog.Warn("failed to create event sink, continuing without", "error", esErr)
		return runDir, nil, false, procLog
	}
	hasRecording := false
	if rcl, ok := client.(interface{ SetDirForRun(string, string) }); ok {
		rcl.SetDirForRun(models.GetRunID(ctx), runDir.Root)
		hasRecording = true
	} else if rcl, ok := client.(interface{ SetDir(string) }); ok {
		rcl.SetDir(runDir.Root)
		hasRecording = true
	}
	runLog, tlErr := newTeeLogger(procLog, runDir.LogPath())
	if tlErr != nil {
		procLog.Warn("failed to create run log, continuing without", "error", tlErr)
		return runDir, eventSink, hasRecording, procLog
	}
	return runDir, eventSink, hasRecording, runLog
}

// buildAgentOptions constructs AgentOptions with model overrides and wires the observer.
func (e *LLMTaskExecutor) buildAgentOptions(ctx context.Context, client proxy.Client, req ExecuteRequest, procLog logging.Logger, capturedEvents *[]any, eventSink *EventSink) assistant.AgentOptions {
	opts := assistant.AgentOptions{
		Logger:       procLog,
		MaxSteps:     assistant.DefaultMaxSteps,
		Guardrails:   e.svc.GuardrailEngine(),
		WorkspaceID:  req.WorkspaceID,
		Channel:      assistant.ChannelAutomation,
		Orchestrator: e.svc.Orchestrator(),
		ModelName:    req.Model,
		Observer: func(ev assistant.AgentEvent) {
			*capturedEvents = append(*capturedEvents, ev)
			// Bound in-memory retention: the full event stream already lives in
			// the run-dir events.jsonl, and recordRun drops the slice. Keeping
			// every chunk of a long report in RAM for the run's duration is
			// wasted memory under concurrent long runs.
			if len(*capturedEvents) > maxCapturedEventsPerRun {
				*capturedEvents = (*capturedEvents)[len(*capturedEvents)-maxCapturedEventsPerRun:]
			}
			e.svc.Events().Publish(req.WorkspaceID, ev)
			if eventSink != nil {
				eventSink.Write(ev)
			}
		},
		GuardrailDecisionHandler: assistant.NewGuardrailDecisionCallback(
			e.svc.GuardrailDecisionStore(),
			func(ev assistant.AgentEvent) {
				e.svc.Events().Publish(req.WorkspaceID, ev)
			},
		),
	}
	if req.Model == "" {
		return opts
	}
	cfg, ok := e.svc.ModelConfig(req.Model)
	if !ok {
		return opts
	}
	if opts.ApplyModelConfig(cfg) {
		tools, listErr := e.svc.ToolProvider().ListTools(ctx)
		if listErr == nil && len(tools) > 0 {
			opts.PlanStrategy = assistant.NewExecutionPlanStrategy(client, tools, procLog)
			procLog.Debug("execution plan strategy enabled", "tools", len(tools))
		}
	}
	procLog.Info("ModelConfig loaded", "model", req.Model, "max_tokens", cfg.MaxTokens, "reasoning_budget", cfg.ReasoningBudget, "context_budget", cfg.ContextBudget, "provider", cfg.Provider)
	return opts
}

// handleAgentError writes run-meta and records the failed run.
func (e *LLMTaskExecutor) handleAgentError(req ExecuteRequest, resp *ExecuteResponse, procLog logging.Logger, execCtx context.Context, runDir *RunDir, capturedEvents []any, startTime time.Time, agErr error) (*ExecuteResponse, error) {
	errStr := fmt.Sprintf("agent execution failed: %v", agErr)
	if runDir != nil {
		meta := RunMeta{
			Model:      req.Model,
			Task:       req.AutomationName,
			DurationMs: time.Since(startTime).Milliseconds(),
			Error:      errStr,
		}
		if t := assistant.GetUsageTracker(execCtx); t != nil {
			meta.LLMCalls = t.LLMCalls
			meta.ToolCalls = t.ToolCalls
		}
		meta.RecordingPath = runDir.RecordingRelPath(e.svc.RecordDir())
		runDir.WriteMeta(meta)
	}
	resp.State.LastError = errStr
	resp.State.SetRunning("")
	e.recordRun(req, resp.State, "", errStr, time.Since(startTime), capturedEvents)
	return resp, fmt.Errorf("agent execution failed: %w", agErr)
}

// handleAgentSuccess formats output, writes per-run artifacts, and records the run.
func (e *LLMTaskExecutor) handleAgentSuccess(req ExecuteRequest, resp *ExecuteResponse, procLog logging.Logger, execCtx context.Context, runDir *RunDir, capturedEvents []any, startTime time.Time, finalReply string, fullHistory []proxy.Message) (*ExecuteResponse, error) {
	runResult := finalReply
	var runError string

	if finalReply == "" && len(fullHistory) > 1 {
		toolCount := 0
		for _, msg := range fullHistory {
			toolCount += len(msg.ToolCalls)
		}
		if toolCount > 0 {
			runResult = fmt.Sprintf("Agent executed %d tool call(s) but returned no final summary.", toolCount)
		} else {
			runResult = "Agent completed the task but returned an empty response."
		}
	}

	// Prepare formatted output
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	header := fmt.Sprintf("[%s] ▶ Executing automation `%s` in workspace `%s`...\nReading task file: `%s`\n\n",
		timestamp, req.AutomationName, req.WorkspaceID, req.TaskFile)

	output := strings.TrimSpace(runResult)

	// Smart Unwrapper: If the model wrapped the entire response in a markdown code block,
	// strip it so our UI can actually render the markdown structure (headers, tables).
	if strings.HasPrefix(output, "```") && strings.HasSuffix(output, "```") {
		lines := strings.Split(output, "\n")
		if len(lines) >= 2 {
			// Remove first and last lines (the backticks)
			output = strings.Join(lines[1:len(lines)-1], "\n")
			output = strings.TrimSpace(output)
		}
	}

	elapsed := time.Since(startTime)
	fullOutput := fmt.Sprintf("%s⏱ **Duration:** %s\n\n### Final Report\n\n%s", header, formatDuration(elapsed), output)
	resp.Output = fullOutput
	resp.State.LastOutput = fullOutput
	runResult = fullOutput
	resp.State.SetRunning("")

	// Push a concluding message to the UI stream to clear "thinking..."
	e.svc.Events().Publish(req.WorkspaceID, assistant.AgentEvent{
		Type:    assistant.EventMessage,
		Channel: assistant.ChannelAutomation,
		Payload: proxy.Message{
			Role:    "system",
			Content: "✔ Execution complete.",
		},
	})

	if runDir != nil {
		runDir.WriteFinalReport(output)
		resultPreview := output
		if len(resultPreview) > 120 {
			resultPreview = resultPreview[:120] + "..."
		}
		meta := RunMeta{
			Model:         req.Model,
			Task:          req.AutomationName,
			DurationMs:    time.Since(startTime).Milliseconds(),
			Result:        resultPreview,
			RecordingPath: runDir.RecordingRelPath(e.svc.RecordDir()),
		}
		if t := assistant.GetUsageTracker(execCtx); t != nil {
			meta.LLMCalls = t.LLMCalls
			meta.ToolCalls = t.ToolCalls
		}
		runDir.WriteMeta(meta)
	}

	e.recordRun(req, resp.State, runResult, runError, time.Since(startTime), capturedEvents)
	return resp, nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s %= 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m %= 60
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

func (e *LLMTaskExecutor) recordRun(req ExecuteRequest, state *models.AgentState, output, errStr string, duration time.Duration, events []any) {
	if state == nil {
		return
	}

	run := models.AutomationRun{
		ID:             generateRunID(),
		WorkspaceID:    req.WorkspaceID,
		AutomationName: req.AutomationName,
		Timestamp:      time.Now(),
		Output:         output,
		Error:          errStr,
		DurationMs:     duration.Milliseconds(),
		Model:          req.Model,
		RecordingRef:   req.RecordingRef,
		Events:         nil, // events live in the run dir, not in state.json
	}

	// Add to full history (capped to last 50 for performance)
	state.History = append(state.History, run)
	if len(state.History) > 30 { // Reduced from 50 to 30 for extra safety
		state.History = state.History[len(state.History)-30:]
	}

	// Update per-automation latest run
	if state.LastRuns == nil {
		state.LastRuns = make(map[string]*models.AutomationRun)
	}
	state.LastRuns[req.AutomationName] = &run
}

func generateRunID() string {
	// Simple unique-ish ID without importing full UUID package if not strictly needed
	// but I checked go.mod and it's there.
	// For now, use timestamp + nano for simplicity if I want to avoid adding the import
	// but let's just use UUID if I can.
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

// annotateTaskWithMemories scans the task content line by line and appends
// a proximity annotation below lines that match a memory entry in FTS5 AND
// share at least one non-stop-word with that entry (word overlap). Capped at
// maxAnnotations to prevent context bloat — 46 annotations added ~4600 chars
// to the prompt, triggering the physical sieve and causing repetition loops.
// Returns the annotated content and the count of annotations added.
func (e *LLMTaskExecutor) annotateTaskWithMemories(ctx context.Context, wsID, taskContent string) (string, int) {
	memStore := e.svc.MemoryStore()
	if memStore == nil {
		return taskContent, 0
	}

	const maxAnnotations = 5
	lines := strings.Split(taskContent, "\n")
	annotated := make([]string, 0, len(lines))
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		annotated = append(annotated, line)

		if trimmed == "" || len(trimmed) < 15 || strings.HasPrefix(trimmed, "---") || count >= maxAnnotations {
			continue
		}

		entries, err := memStore.Search(ctx, wsID, trimmed, 1)
		if err != nil || len(entries) == 0 {
			continue
		}

		entry := entries[0]

		// Word overlap check: at least one non-stop-word in the task line must
		// also appear in the memory entry's title or content. This filters out
		// coincidental FTS5 matches on common words ("Step 1: list directory"
		// matching a memory about compliance-audit because both happen to
		// contain "file").
		if !wordsOverlap(trimmed, entry.Title+" "+entry.Content) {
			continue
		}

		preview := entry.Content
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		annotated = append(annotated, fmt.Sprintf("  ↳ [Memory: %s — %s]", entry.Title, preview))
		count++
	}

	return strings.Join(annotated, "\n"), count
}

var annotationStopWords = map[string]bool{
	"step": true, "task": true, "run": true, "the": true, "a": true, "an": true,
	"and": true, "or": true, "to": true, "in": true, "of": true, "for": true,
	"with": true, "on": true, "by": true, "at": true, "is": true, "be": true,
	"do": true, "not": true, "are": true, "was": true, "will": true, "can": true,
}

// wordsOverlap returns true when the two strings share at least one non-stop-
// word (case-insensitive). Used by annotateTaskWithMemories to filter out
// coincidental FTS5 matches.
func wordsOverlap(a, b string) bool {
	wordsA := extractNonStopWords(a)
	if len(wordsA) == 0 {
		return false
	}
	set := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		set[w] = true
	}
	for _, w := range extractNonStopWords(b) {
		if set[w] {
			return true
		}
	}
	return false
}

// extractNonStopWords lowercases and splits a string into non-stop-word tokens.
func extractNonStopWords(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			if cur.Len() > 0 {
				w := strings.ToLower(cur.String())
				if !annotationStopWords[w] {
					words = append(words, w)
				}
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		w := strings.ToLower(cur.String())
		if !annotationStopWords[w] {
			words = append(words, w)
		}
	}
	return words
}

// buildPriorRunSeededHistory extracts tool call and result pairs from the last
// automation run's event log and converts them to proxy.Message format for
// injection into the initial history. Seeds from any prior run with events,
// even if the run had an error — completed tool calls are still valid and the
// model naturally skips steps already in its conversation history.
func (e *LLMTaskExecutor) buildPriorRunSeededHistory(req ExecuteRequest) []proxy.Message {
	if req.State == nil {
		return nil
	}
	lastRun, ok := req.State.LastRuns[req.AutomationName]
	if !ok || lastRun == nil || len(lastRun.Events) == 0 {
		return nil
	}

	var seeded []proxy.Message
	for _, ev := range lastRun.Events {
		aev, ok := ev.(assistant.AgentEvent)
		if !ok {
			continue
		}
		switch aev.Type {
		case assistant.EventToolCall:
			tc, ok := aev.Payload.(proxy.ToolCall)
			if !ok {
				continue
			}
			seeded = append(seeded, proxy.Message{
				Role:      proxy.AssistantRole,
				ToolCalls: []proxy.ToolCall{tc},
			})
		case assistant.EventToolResult:
			payload, ok := aev.Payload.(map[string]any)
			if !ok {
				continue
			}
			id, _ := payload["id"].(string)
			if id == "" {
				continue
			}
			// The result can be any type (string, number, map). Format it
			// as a string so the model sees the tool output verbatim.
			seeded = append(seeded, proxy.Message{
				Role:       proxy.ToolRole,
				Content:    fmt.Sprintf("%v", payload["result"]),
				ToolCallID: id,
			})
		}
	}
	return seeded
}

func (e *LLMTaskExecutor) buildPrompt(taskContent string, req ExecuteRequest) string {
	return fmt.Sprintf(prompts.AutomationTaskPrompt,
		req.WorkspaceID, req.TaskFile, taskContent)
}


// ============================================================================
// Pulse Logic (Smart Skip)
// ============================================================================

const heartbeatOKMarker = "HEARTBEAT_OK"

func isHeartbeatOK(output string) bool {
	return strings.Contains(output, heartbeatOKMarker)
}

// ApplyPulseLogic applies the Smart Skip logic to suppress noisy heartbeat output.
func ApplyPulseLogic(resp *ExecuteResponse) {
	if isHeartbeatOK(resp.Output) {
		resp.Output = ""
		resp.State.LastPulse = time.Now()
	}
}

// pruneEvents strips heavy payloads from the event history before persistence.
func pruneEvents(events []any) []any {
	if len(events) == 0 {
		return events
	}
	pruned := make([]any, 0, len(events))
	for _, ev := range events {
		// We only keep a lightweight version of events for history.
		// Detailed payloads (like file contents) are truncated.
		switch v := ev.(type) {
		case assistant.AgentEvent:
			if v.Type == assistant.EventMessage {
				msg, ok := v.Payload.(proxy.Message)
				if ok {
					// Truncate message content to 1KB for history
					if len(msg.Content) > 1024 {
						msg.Content = msg.Content[:1024] + "... [truncated for history]"
					}
					v.Payload = msg
				}
			}
			pruned = append(pruned, v)
		default:
			pruned = append(pruned, ev)
		}
	}
	return pruned
}
