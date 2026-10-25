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
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/run"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
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
	// Profile is the prompt profile id; "" runs the default for automations.
	Profile string
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
	run.OptionDeps
	ClientProvider() proxy.LLMClientProvider
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	// Profiles is the prompt-profile catalog a run's behaviour is resolved from.
	Profiles() *profile.Catalog
	EffectiveToolCallFormat(ctx context.Context, modelName string) string
	Logger() logging.Logger
	ToolProvider() assistant.ToolProvider
	Engine() assistant.Engine
	ProcessLogger(workspaceID string) logging.Logger
	Persistence() *persistence.WorkspaceManager
	Events() assistant.EventPublisher
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
	reports   ReportHook
}

// ReportHook lets a feature add facts it collected during a run to the run's final report, without the executor
// knowing what the feature is. Begin returns the context the run executes in (so the feature can collect from tools);
// Augment returns the report to keep, given the one the agent wrote.
type ReportHook interface {
	Begin(ctx context.Context) context.Context
	Augment(ctx context.Context, workspaceID, automation, report string) string
}

// SetReportHook installs the report hook; nil (the default) leaves reports as the agent wrote them.
func (e *LLMTaskExecutor) SetReportHook(h ReportHook) {
	e.reports = h
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

	execCtx := usage.WithTracker(runContextFor(ctx, req, models.NewRunID()))
	if e.reports != nil {
		execCtx = e.reports.Begin(execCtx)
	}

	rec := e.recording(execCtx, client, req, procLog)
	runDir, eventSink := rec.Dir, rec.Events

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
	runProfile := e.svc.Profiles().Resolve(profile.KindAutomation, req.Profile)
	agentOpts := e.buildAgentOptions(req, runProfile, procLog, eventSink)
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

	history := []proxy.Message{
		{Role: "system", Content: assistant.BuildSystemPrompt(e.svc.Persistence(), req.WorkspaceID, runProfile, useNativeTools)},
		{Role: "user", Content: e.buildPrompt(req.TaskContent, req, runProfile)},
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

// recording opens the run's recording: its folder, event file and the client's request log (all empty when run
// logging is off or the run has no model).
func (e *LLMTaskExecutor) recording(ctx context.Context, client proxy.Client, req ExecuteRequest, procLog logging.Logger) run.Recording {
	return run.OpenRecording(client, run.RecordingSpec{
		Enabled: e.svc.RunLoggingEnabled(), Root: e.svc.RootDir(), WorkspaceID: req.WorkspaceID,
		Task: req.AutomationName, Model: req.Model, RunID: models.GetRunID(ctx), Log: procLog,
	})
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

// buildAgentOptions resolves the run's agent options (the one resolver chat uses too) and wires the observer.
func (e *LLMTaskExecutor) buildAgentOptions(req ExecuteRequest, runProfile profile.Profile, procLog logging.Logger, eventSink *eventbus.Sink) assistant.AgentOptions {
	spec := runSpec(req, runProfile)
	spec.Logger = procLog
	spec.Observer = func(ev assistant.AgentEvent) {
		// The full event stream already lives in the run-dir events.jsonl;
		// this observer only fans out to live subscribers and the sink.
		e.publishRunEvent(req, ev)
		if eventSink != nil {
			eventSink.Write(ev)
		}
	}
	// Approval events go to live subscribers only, not the run's event file.
	spec.DecisionEvents = func(ev assistant.AgentEvent) { e.publishRunEvent(req, ev) }
	return run.ResolveOptions(e.svc, spec)
}

// runSpec is the automation's choices for its run: its model, profile, loop, allowed tools and memory mode. The
// allow list restricts the exposed tool schema for unattended runs (allow ∩ guardrail-disabled, resolved in
// NewAgent); a profile can hide tools, never add one the list lacks.
func runSpec(req ExecuteRequest, runProfile profile.Profile) run.Spec {
	return run.Spec{
		Kind: profile.KindAutomation, WorkspaceID: req.WorkspaceID, Model: req.Model, Profile: runProfile,
		Overrides: run.Overrides{
			LoopStrategy: assistant.LoopStrategyName(req.LoopStrategy),
			AllowedTools: allowedToolsFor(req),
			Memory:       req.MemoryMode,
		},
	}
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
	// reportPath is where the run's final report was saved in the workspace; empty when none was.
	reportPath string
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

	output = e.augmentReport(ctx, outcome.req, output)

	elapsed := time.Since(outcome.startTime)
	fullOutput := fmt.Sprintf("%s⏱ **Duration:** %s\n\n### Final Report\n\n%s", header, formatDuration(elapsed), output)
	outcome.resp.Output = fullOutput
	outcome.resp.Report = output
	runResult = fullOutput
	outcome.resp.State.SetRunning("")

	// Push a concluding message to the UI stream to clear "thinking..."
	e.publishRunEvent(outcome.req, assistant.AgentEvent{
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

	if finalReply != "" {
		outcome.reportPath = e.keepFinalReport(outcome, output)
	}

	e.recordRun(outcome, runResult, runError, time.Since(outcome.startTime))
	return outcome.resp, nil
}

// augmentReport lets the report hook add what it collected during the run; without a hook the report is the agent's own.
func (e *LLMTaskExecutor) augmentReport(ctx context.Context, req ExecuteRequest, report string) string {
	if e.reports == nil {
		return report
	}
	return e.reports.Augment(ctx, req.WorkspaceID, req.AutomationName, report)
}

// reportWriter saves a run's final report in the workspace and returns the path it used.
// *persistence.WorkspaceManager satisfies it.
type reportWriter interface {
	WriteReport(workspaceID, automation, runID string, at time.Time, content string) (string, error)
}

// saveFinalReport keeps a run's final report as a file in the workspace, so the next run of the same automation does not
// replace it and the 30-run history does not push it out. It returns the saved path, or "" when there is nothing worth
// keeping: no reply from the agent, or a heartbeat that found nothing to report. Credential-shaped text is masked first,
// as the report becomes a new file the agent can read in a later run.
func saveFinalReport(w reportWriter, outcome runOutcome, report string) (string, error) {
	if strings.TrimSpace(report) == "" || isQuietResponse(outcome.resp) {
		return "", nil
	}
	return w.WriteReport(outcome.req.WorkspaceID, outcome.req.AutomationName, outcome.runID, outcome.startTime, tools.RedactForRecording(report))
}

// keepFinalReport saves the report of a finished run, logs where it went and returns that path ("" when nothing was
// saved). A failure to save is logged, never fatal: the run itself succeeded and its report is still in the run history.
func (e *LLMTaskExecutor) keepFinalReport(outcome runOutcome, report string) string {
	store := e.svc.Persistence()
	if store == nil {
		return ""
	}
	log := e.svc.ProcessLogger(outcome.req.WorkspaceID)
	path, err := saveFinalReport(store, outcome, report)
	if err != nil {
		log.Warn("failed to save the final report", "automation", outcome.req.AutomationName, "error", err)
		return ""
	}
	if path != "" {
		log.Info("Final report saved", "automation", outcome.req.AutomationName, "path", path)
	}
	return path
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
		runID = models.NewRunID() // runs that failed before their context was built
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
		ReportPath:     outcome.reportPath,
		Events:         nil, // events live in the run dir, not in state.json
		Warnings:       outcome.warnings,
	}

	state.AppendRun(run)
	outcome.resp.RunID = runID
}

func (e *LLMTaskExecutor) buildPrompt(taskContent string, req ExecuteRequest, runProfile profile.Profile) string {
	prompt := fmt.Sprintf(prompts.AutomationTaskPrompt,
		req.WorkspaceID, req.TaskFile, taskContent)
	if run.HotMemoryActive(e.svc, runSpec(req, runProfile)) && toolOffered(req, runProfile, models.ToolMemorySearch) {
		prompt += prompts.AutomationMemoryBlock()
	}
	return prompt
}

// toolOffered reports whether the run exposes a tool: the resolved allow-list includes it (no list means every
// tool) and the run's profile does not hide it.
func toolOffered(req ExecuteRequest, runProfile profile.Profile, name string) bool {
	allowed := allowedToolsFor(req)
	return (len(allowed) == 0 || slices.Contains(allowed, name)) && !slices.Contains(runProfile.ExcludedTools(), name)
}

// publishRunEvent sends an event of this run, naming its automation so it is routed
// to that automation's stream (see Dispatcher.publish).
func (e *LLMTaskExecutor) publishRunEvent(req ExecuteRequest, ev assistant.AgentEvent) {
	ev.Automation = req.AutomationName
	e.svc.Events().Publish(req.WorkspaceID, ev)
}
