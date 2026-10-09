package automation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/rundir"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

// Sentinel errors for shell PGID queries from the automation executor.
var (
	ErrShellPGIDNotAvailable = errors.New("shell PGID not available")
	ErrNoShellPool           = errors.New("shell pool not available")
)

// TaskExecutor executes automations.
type TaskExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
	// ShellPGID returns the negated process group ID for a workspace's
	// active shell session. Returns an error when not available.
	ShellPGID(ctx context.Context, workspaceID string) (int, error)
	// ModelTimeout returns the pinned model's timeout_minutes as a duration,
	// or 0 when the model is unknown or has no explicit timeout — the
	// dispatcher uses it to let a model's timeout override its own cap.
	ModelTimeout(modelName string) time.Duration
}

type ExecuteRequest struct {
	WorkspaceID    string
	AutomationName string
	TaskFile       string
	TaskContent    string
	Strategy       ExecutionStrategy
	State          *models.AgentState
	Model          string   // Optional model override
	LoopStrategy   string   // Optional per-run loop archetype override; "" = model config default
	AllowedTools   []string // restrict tools for unattended runs
	RecordingRef   string   // Recording file ID for playback (empty = live LLM)
	// NetworkGrant is the automation's per-run network scope override (L2).
	// Empty = inherit the workspace scope. Resolved against host (L0) + merged
	// workspace guardrails (L1) in Execute via GuardrailEngine.ResolveRunScope.
	NetworkGrant models.NetworkScope
	// MemoryMode overrides the global automation hot-memory default; "" inherits it.
	MemoryMode models.MemoryMode
	// Journal enables the automation_journal tool for this run.
	Journal bool
}

type ExecuteResponse struct {
	// RunID is the id of the history entry this run recorded, so the dispatcher can annotate that run afterwards.
	RunID  string
	Output string
	// Report is the agent's final report alone (no run header), the text a
	// notify delivery is built from. Empty when the run produced none.
	Report string
	Error  error
	State  *models.AgentState
}

// LLMServiceProvider is the minimal interface needed by LLMTaskExecutor.
// It avoids an import cycle by not depending on the api or nodeherder packages.
type LLMServiceProvider interface {
	ClientProvider() proxy.LLMClientProvider
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	ModelConfig(modelName string) (models.ModelConfig, bool)
	EffectiveToolCallFormat(ctx context.Context, modelName string) string
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
	MemorySettings() *models.MemoryConfig
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

func (e *DefaultTaskExecutor) ModelTimeout(modelName string) time.Duration { return 0 }

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

// ModelTimeout resolves the pinned model's timeout_minutes so the dispatcher
// can let a model's timeout override its own run cap.
func (e *LLMTaskExecutor) ModelTimeout(modelName string) time.Duration {
	if cfg, ok := e.svc.ModelConfig(modelName); ok && cfg.TimeoutMinutes > 0 {
		return time.Duration(cfg.TimeoutMinutes) * time.Minute
	}
	return 0
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

	execCtx := usage.WithTracker(runContextFor(ctx, req, generateRunID()))

	runDir, eventSink, _ := e.setupRunDir(execCtx, client, req, procLog)

	// Resolve the effective tool-call format (explicit override, cloud native
	// default, or a cached endpoint capability probe for local models) BEFORE
	// building the agent, so a newly added local model does not require a
	// per-model tool_call_format setting to use native function calling.
	//
	// Ordering matters: the probe can take seconds against a local endpoint
	// (it is a real HTTP round-trip), and buildAgentOptions → ApplyModelConfig
	// locks UseNativeTools from the stored config at agent-build time. Resolving
	// after NewAgent (as before) meant the FIRST run after a backend restart —
	// when the probe cache is cold — built the agent in XML text mode, sent no
	// tools array, and the model's XML tool calls failed to parse (observed
	// 2026-08-31 14:26 run: 5/15 tool calls, Steps 4-9 unreached). Resolving
	// first persists "native" onto the manager's stored config, so the agent is
	// built with native tool calling from turn one.
	useNativeTools := e.svc.EffectiveToolCallFormat(ctx, req.Model) == "native"

	toolProvider := e.svc.ToolProvider()
	agentOpts := e.buildAgentOptions(req, procLog, eventSink)
	// Resolve the per-run network scope ONCE (host ceiling ∩ workspace L1,
	// overridden by the automation's L2 grant) and reuse it everywhere for this
	// run — agent schema, run ctx stamp, shell pool key, and the run-meta audit
	// trail — so no two consumers can resolve different scopes. Chat runs never
	// set this (inherit).
	runScope := e.runNetworkScope(req)
	agentOpts.RunNetworkScope = runScope
	procLog.Info("Resolved run network scope", "workspace", req.WorkspaceID, "network_scope", runScope, "grant", req.NetworkGrant)
	outcome := runOutcome{
		req:       req,
		resp:      resp,
		runDir:    runDir,
		startTime: startTime,
		runScope:  runScope,
		runID:     models.GetRunID(execCtx),
	}
	agent := assistant.NewAgent(client, toolProvider, e.svc.Engine(), agentOpts)

	agentsFileContent := assistant.LoadAgentsFile(e.svc.Persistence(), req.WorkspaceID)
	systemPrompt := prompts.AssembleSystemPrompt(agentsFileContent, useNativeTools)

	history := []proxy.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: e.buildPrompt(req.TaskContent, req)},
	}

	finalReply, fullHistory, agErr := agent.Execute(execCtx, history)
	outcome.warnings = agent.ToolWarnings()

	if eventSink != nil {
		eventSink.Close()
	}
	if rcl, ok := client.(interface{ CloseRun(string) }); ok {
		rcl.CloseRun(models.GetRunID(execCtx))
	}

	if t := usage.FromContext(execCtx); t != nil {
		procLog.Info("Usage", "llm_calls", t.LLMCalls, "tool_calls", t.ToolCalls,
			"input_tokens", t.InputTokens, "output_tokens", t.OutputTokens)
	}

	if agErr != nil {
		return e.handleAgentError(execCtx, outcome, agErr)
	}

	return e.handleAgentSuccess(execCtx, outcome, finalReply, fullHistory)
}

func runDirName(runDir *rundir.RunDir) string {
	if runDir == nil {
		return ""
	}
	return filepath.Base(runDir.Root)
}

// getLLMClient returns the LLM client for a live model or playback recording.
func (e *LLMTaskExecutor) getLLMClient(ctx context.Context, req ExecuteRequest, resp *ExecuteResponse, startTime time.Time) (proxy.Client, error) {
	if req.RecordingRef != "" {
		client, err := e.svc.GetPlaybackClient(ctx, req.RecordingRef)
		if err != nil {
			errStr := fmt.Sprintf("failed to load recording %s: %v", req.RecordingRef, err)
			resp.State.SetRunning("")
			e.recordRun(runOutcome{req: req, resp: resp, startTime: startTime}, "", errStr, time.Since(startTime))
			return nil, fmt.Errorf("failed to load recording: %w", err)
		}
		procLog := e.svc.ProcessLogger(req.WorkspaceID)
		procLog.Info("Running automation from recording", "recording", req.RecordingRef)
		return client, nil
	}

	var client proxy.Client
	var err error
	var get func() (proxy.Client, error)
	if req.Model != "" {
		name := req.Model
		get = func() (proxy.Client, error) { return e.svc.GetClientForModel(ctx, name) }
	} else {
		get = func() (proxy.Client, error) { return e.svc.ClientProvider().GetClient(ctx) }
	}
	// A cold local model returns ErrModelStarting while llama-server warms up.
	// Poll (bounded) instead of failing the unattended run on the first try, so
	// a midnight automation can auto-start the local LLM and wait for it.
	client, err = proxy.WaitForClient(ctx, get, proxy.ModelWait{
		ModelName: req.Model, PollInterval: models.ModelStartPollInterval, Timeout: models.ModelStartWaitTimeout,
	})
	if err != nil {
		errStr := fmt.Sprintf("failed to get llm client: %v", err)
		resp.State.SetRunning("")
		e.recordRun(runOutcome{req: req, resp: resp, startTime: startTime}, "", errStr, time.Since(startTime))
		return nil, fmt.Errorf("failed to get llm client: %w", err)
	}
	return client, nil
}

func (e *LLMTaskExecutor) setupRunDir(ctx context.Context, client proxy.Client, req ExecuteRequest, procLog logging.Logger) (*rundir.RunDir, *eventbus.Sink, bool) {
	if !e.svc.RunLoggingEnabled() {
		return nil, nil, false
	}
	rootDir := e.svc.RootDir()
	if rootDir == "" || req.Model == "" {
		return nil, nil, false
	}
	parent := filepath.Join(rootDir, "runs")
	runDir, rErr := rundir.NewRunDir(parent, req.WorkspaceID, req.AutomationName, req.Model)
	if rErr != nil {
		procLog.Warn("failed to create run dir, continuing without per-run output", "error", rErr)
		return nil, nil, false
	}
	eventSink, esErr := eventbus.NewSink(runDir.EventsPath())
	if esErr != nil {
		procLog.Warn("failed to create event sink, continuing without", "error", esErr)
		return runDir, nil, false
	}
	hasRecording := false
	if rcl, ok := client.(interface{ SetDirForRun(string, string) }); ok {
		rcl.SetDirForRun(models.GetRunID(ctx), runDir.Root)
		hasRecording = true
	} else if rcl, ok := client.(interface{ SetDir(string) }); ok {
		rcl.SetDir(runDir.Root)
		hasRecording = true
	}
	return runDir, eventSink, hasRecording
}

// runNetworkScope resolves the effective per-run network scope (host ∩ merged
// workspace L1, overridden by the automation's L2 grant). Idempotent and cheap;
// used for the agent schema/ctx stamp and the run-meta audit trail.
func (e *LLMTaskExecutor) runNetworkScope(req ExecuteRequest) models.NetworkScope {
	return e.svc.GuardrailEngine().ResolveRunScope(req.WorkspaceID, req.NetworkGrant)
}

// runContextFor is the run context for one request: the run identity plus, for
// an automation that keeps a journal, the flag that makes the journal tool
// visible and callable.
func runContextFor(ctx context.Context, req ExecuteRequest, runID string) context.Context {
	ctx = newRunContext(ctx, req.AutomationName, runID)
	if req.Journal {
		ctx = models.WithJournalRun(ctx)
	}
	return ctx
}

// allowedToolsFor is the request's tool allowlist. An automation that restricts
// its tools still gets the journal tool when the journal is on; no allowlist
// stays unrestricted (the journal tool is then scoped by the run context alone).
func allowedToolsFor(req ExecuteRequest) []string {
	if len(req.AllowedTools) == 0 || !req.Journal {
		return req.AllowedTools
	}
	return append(slices.Clone(req.AllowedTools), models.ToolAutomationJournal)
}

// newRunContext stamps a run's identity and marks it unattended: no operator is
// present, so tools that persist state choose conservative defaults.
func newRunContext(ctx context.Context, automationName, runID string) context.Context {
	ctx = models.WithTaskName(ctx, automationName)
	ctx = models.WithRunID(ctx, runID)
	return models.WithUnattendedRun(ctx)
}

// buildAgentOptions constructs AgentOptions with model overrides and wires the observer.
func (e *LLMTaskExecutor) buildAgentOptions(req ExecuteRequest, procLog logging.Logger, eventSink *eventbus.Sink) assistant.AgentOptions {
	opts := assistant.AgentOptions{
		Logger:       procLog,
		MaxSteps:     assistant.DefaultMaxSteps,
		Guardrails:   e.svc.GuardrailEngine(),
		WorkspaceID:  req.WorkspaceID,
		Channel:      assistant.ChannelAutomation,
		Orchestrator: e.svc.Orchestrator(),
		ModelName:    req.Model,
		Observer: func(ev assistant.AgentEvent) {
			// The full event stream already lives in the run-dir events.jsonl;
			// this observer only fans out to live subscribers and the sink.
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
			assistant.ChannelAutomation,
		),
		// AllowedTools restricts the exposed tool schema for unattended runs
		// (allow ∩ guardrail-disabled, resolved in NewAgent).
		AllowedTools: allowedToolsFor(req),
	}
	if e.memoryActive(req) {
		opts.MemoryStore = e.svc.MemoryStore()
		opts.EnableHotMemory = true
	}
	if req.Model == "" {
		return opts
	}
	cfg, ok := e.svc.ModelConfig(req.Model)
	if !ok {
		return opts
	}
	opts.ApplyModelConfig(cfg)
	// Per-run loop-strategy override: applied AFTER the model config so the
	// automation wins for this run only. Unknown non-empty values are rejected
	// at the automation/template handler boundary (400); ParseLoopStrategy is
	// defense-in-depth here.
	if req.LoopStrategy != "" {
		opts.LoopStrategy = assistant.ParseLoopStrategy(assistant.LoopStrategyName(req.LoopStrategy))
	}
	procLog.Info("ModelConfig loaded", "model", req.Model, "max_tokens", cfg.MaxTokens, "reasoning_budget", cfg.ReasoningBudget, "context_budget", cfg.ContextBudget, "provider", cfg.Provider)
	return opts
}

// runOutcome is the per-run state the terminal writers (error/success) need.
// Grouped into one struct so the writers take ≤3 parameters — the previous
// 9-positional-argument signatures were order-sensitive and unreadable (rule:
// function params ≤4, else an options/deps struct).
type runOutcome struct {
	req       ExecuteRequest
	resp      *ExecuteResponse
	runDir    *rundir.RunDir
	startTime time.Time
	runScope  models.NetworkScope
	// runID is the id the run context carries; the recorded run uses it too.
	runID string
	// warnings holds non-fatal tool failures from the agent (e.g. a delivery
	// connector being down), persisted into run-meta and the run ledger.
	warnings []string
}

// handleAgentError writes run-meta and records the failed run.
func (e *LLMTaskExecutor) handleAgentError(ctx context.Context, outcome runOutcome, agErr error) (*ExecuteResponse, error) {
	errStr := fmt.Sprintf("agent execution failed: %v", agErr)
	if outcome.runDir != nil {
		meta := rundir.RunMeta{
			Model:        outcome.req.Model,
			Task:         outcome.req.AutomationName,
			DurationMs:   time.Since(outcome.startTime).Milliseconds(),
			Error:        errStr,
			NetworkScope: string(outcome.runScope),
			Warnings:     outcome.warnings,
		}
		if t := usage.FromContext(ctx); t != nil {
			meta.LLMCalls = t.LLMCalls
			meta.ToolCalls = t.ToolCalls
		}
		meta.RecordingPath = outcome.runDir.RecordingRelPath(e.svc.RecordDir())
		outcome.runDir.WriteMeta(meta)
	}
	outcome.resp.State.SetRunning("")
	e.recordRun(outcome, "", errStr, time.Since(outcome.startTime))
	return outcome.resp, fmt.Errorf("agent execution failed: %w", agErr)
}

// handleAgentSuccess formats output, writes per-run artifacts, and records the run.
func (e *LLMTaskExecutor) handleAgentSuccess(ctx context.Context, outcome runOutcome, finalReply string, fullHistory []proxy.Message) (*ExecuteResponse, error) {
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
		timestamp, outcome.req.AutomationName, outcome.req.WorkspaceID, outcome.req.TaskFile)

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

	elapsed := time.Since(outcome.startTime)
	fullOutput := fmt.Sprintf("%s⏱ **Duration:** %s\n\n### Final Report\n\n%s", header, formatDuration(elapsed), output)
	outcome.resp.Output = fullOutput
	outcome.resp.Report = output
	runResult = fullOutput
	outcome.resp.State.SetRunning("")

	// Push a concluding message to the UI stream to clear "thinking..."
	e.svc.Events().Publish(outcome.req.WorkspaceID, assistant.AgentEvent{
		Type:    assistant.EventMessage,
		Channel: assistant.ChannelAutomation,
		Payload: proxy.Message{
			Role:    "system",
			Content: assistant.MsgExecutionComplete,
		},
	})

	if outcome.runDir != nil {
		outcome.runDir.WriteFinalReport(output)
		resultPreview := output
		if len(resultPreview) > 120 {
			resultPreview = resultPreview[:120] + "..."
		}
		meta := rundir.RunMeta{
			Model:         outcome.req.Model,
			Task:          outcome.req.AutomationName,
			DurationMs:    time.Since(outcome.startTime).Milliseconds(),
			Result:        resultPreview,
			RecordingPath: outcome.runDir.RecordingRelPath(e.svc.RecordDir()),
			NetworkScope:  string(outcome.runScope),
			Warnings:      outcome.warnings,
		}
		if t := usage.FromContext(ctx); t != nil {
			meta.LLMCalls = t.LLMCalls
			meta.ToolCalls = t.ToolCalls
		}
		outcome.runDir.WriteMeta(meta)
	}

	e.recordRun(outcome, runResult, runError, time.Since(outcome.startTime))
	return outcome.resp, nil
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

func (e *LLMTaskExecutor) recordRun(outcome runOutcome, output, errStr string, duration time.Duration) {
	state := outcome.resp.State
	if state == nil {
		return
	}
	req := outcome.req

	runID := outcome.runID
	if runID == "" {
		runID = generateRunID() // runs that failed before their context was built
	}
	run := models.AutomationRun{
		ID:             runID,
		WorkspaceID:    req.WorkspaceID,
		AutomationName: req.AutomationName,
		Timestamp:      time.Now(),
		Output:         output,
		Error:          errStr,
		DurationMs:     duration.Milliseconds(),
		Model:          req.Model,
		RecordingRef:   req.RecordingRef,
		RunDirName:     runDirName(outcome.runDir),
		Events:         nil, // events live in the run dir, not in state.json
		Warnings:       outcome.warnings,
	}

	state.AppendRun(run)
	outcome.resp.RunID = runID
}

func generateRunID() string {
	// Simple unique-ish ID without importing full UUID package if not strictly needed
	// but I checked go.mod and it's there.
	// For now, use timestamp + nano for simplicity if I want to avoid adding the import
	// but let's just use UUID if I can.
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

// memoryActive reports whether this run has memory: the automation's mode resolved against the
// global automation default, and a store to search. The prompt and the agent options share it.
func (e *LLMTaskExecutor) memoryActive(req ExecuteRequest) bool {
	return req.MemoryMode.Effective(e.svc.MemorySettings().AutomationHotDefault()) && e.svc.MemoryStore() != nil
}

func (e *LLMTaskExecutor) buildPrompt(taskContent string, req ExecuteRequest) string {
	prompt := fmt.Sprintf(prompts.AutomationTaskPrompt,
		req.WorkspaceID, req.TaskFile, taskContent)
	if e.memoryActive(req) && toolOffered(req, models.ToolMemorySearch) {
		prompt += prompts.AutomationMemoryBlock()
	}
	return prompt
}

// toolOffered reports whether the run's resolved allow-list includes a tool (no list means every tool).
func toolOffered(req ExecuteRequest, name string) bool {
	allowed := allowedToolsFor(req)
	return len(allowed) == 0 || slices.Contains(allowed, name)
}

// ============================================================================
// Pulse Logic (Smart Skip)
// ============================================================================

const (
	heartbeatOKMarker = "HEARTBEAT_OK"
	// heartbeatMarkupChars are markdown emphasis characters a model may wrap
	// around the marker (**HEARTBEAT_OK**, `HEARTBEAT_OK`).
	heartbeatMarkupChars = "*_`# \t\r\n"
)

// isQuietHeartbeat reports whether a report is the "nothing to report" signal:
// it must START with the marker (ignoring markdown emphasis), so a real alert
// that merely mentions the marker is never swallowed. The UI's smart skip and
// connector delivery share this one definition.
func isQuietHeartbeat(report string) bool {
	return strings.HasPrefix(strings.TrimLeft(report, heartbeatMarkupChars), heartbeatOKMarker)
}

// isQuietResponse judges the agent's report alone when there is one — Output
// carries a run header and would match a marker mentioned anywhere. A response
// without a Report (legacy executors) falls back to scanning Output.
func isQuietResponse(resp *ExecuteResponse) bool {
	if resp.Report != "" {
		return isQuietHeartbeat(resp.Report)
	}
	return strings.Contains(resp.Output, heartbeatOKMarker)
}

// heartbeatResultOf classifies a finished heartbeat check by its report.
func heartbeatResultOf(resp *ExecuteResponse) models.HeartbeatResult {
	if isQuietResponse(resp) {
		return models.HeartbeatQuiet
	}
	return models.HeartbeatAlert
}

// ApplyPulseLogic applies the Smart Skip logic to suppress noisy heartbeat output.
func ApplyPulseLogic(resp *ExecuteResponse) {
	if isQuietResponse(resp) {
		resp.Output = ""
		resp.State.LastPulse = time.Now()
	}
}
