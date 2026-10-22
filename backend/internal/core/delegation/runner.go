// Package delegation runs delegated tasks as sub-agents: child runs on a cloud model, each from a helper profile, that
// read the web and return a summary. It implements assistant.SubRunner and is injected at bootstrap; assistant never
// imports it.
package delegation

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

const (
	// slotWait is how long a task waits for a sub-agent slot before it is reported busy.
	slotWait = 2 * time.Minute
	// eventDetailChars bounds what one progress event carries of a tool's arguments or result.
	eventDetailChars = 160
)

// childTools is everything a v1 helper may call: the web, and nothing on this machine or its network.
var childTools = []string{models.ToolInternetSearch, models.ToolNetworkFetch}

// Deps is what running a child needs. It is the app's services as they were before the runner was added to them, so
// their own SubRunner is nil and a child cannot delegate in turn.
type Deps interface {
	assistant.RunOptionDeps
	Profiles() *profile.Catalog
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	EffectiveToolCallFormat(ctx context.Context, modelName string) string
	ToolProvider() assistant.ToolProvider
	Engine() assistant.Engine
	LaneKeyFor(model string) runlane.LaneKey
	ProcessLogger(workspaceID string) logging.Logger
}

// Runner runs delegated tasks.
type Runner struct {
	deps     Deps
	pool     *pool
	slotWait time.Duration
}

// New returns a runner over the app's services; the pool size is read from the settings on every task.
func New(deps Deps) *Runner {
	return &Runner{deps: deps, pool: newPool(func() int { return deps.AgentSettings().SubagentLimit() }), slotWait: slotWait}
}

var _ assistant.SubRunner = (*Runner)(nil)

// Enabled reports whether a sub-agent model is named and it is a cloud model.
func (r *Runner) Enabled() bool {
	_, err := r.model()
	return err == nil
}

// model is the configured sub-agent model, refused unless the lane classifier says cloud. A model it cannot resolve
// counts as local, so a child never lands on the GPU a local parent holds.
func (r *Runner) model() (string, error) {
	name := r.deps.AgentSettings().SubagentModelName()
	if name == "" {
		return "", errors.New("no sub-agent model is set (Settings → agent.subagent_model)")
	}
	if r.deps.LaneKeyFor(name) != runlane.LaneCloud {
		return "", fmt.Errorf("the sub-agent model %q is not a cloud model", name)
	}
	return name, nil
}

// Run runs one task to its end. Every outcome is a status: a busy pool, a refused model or a failed child never
// becomes an error for the parent.
func (r *Runner) Run(ctx context.Context, req assistant.SubRunRequest) assistant.SubRunResult {
	start := time.Now()
	res := assistant.SubRunResult{RunID: models.NewRunID(), Status: assistant.SubRunRefused}
	model, err := r.model()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Model = model
	helper, ok := r.deps.Profiles().Get(req.Agent)
	if !ok {
		res.Error = fmt.Sprintf("no helper profile %q", req.Agent)
		return res
	}
	release, err := r.pool.acquire(ctx, r.slotWait)
	if err != nil {
		res.Status = stoppedOr(ctx, assistant.SubRunBusy)
		return res
	}
	defer release()

	emit(req, res.RunID, assistant.SubagentStarted, "", req.Task)
	r.runChild(ctx, req, helper, &res)
	emit(req, res.RunID, assistant.SubagentDone, "", string(res.Status))
	res.DurationMs = time.Since(start).Milliseconds()
	return res
}

// runChild runs the helper's agent and fills in the result: its answer, its status and the tokens its provider
// reported, which go to the child's own tracker, not the parent's.
func (r *Runner) runChild(ctx context.Context, req assistant.SubRunRequest, helper profile.Profile, res *assistant.SubRunResult) {
	client, err := proxy.WaitForClient(ctx, func() (proxy.Client, error) { return r.deps.GetClientForModel(ctx, res.Model) }, proxy.ModelWait{
		ModelName: res.Model, PollInterval: models.ModelStartPollInterval, Timeout: models.ModelStartWaitTimeout,
	})
	if err != nil {
		res.Status, res.Error = stoppedOr(ctx, assistant.SubRunFailed), err.Error()
		return
	}
	childCtx := models.WithUnattendedRun(models.WithRunID(usage.WithNewTracker(ctx), res.RunID))
	defer closeRun(client, res.RunID)

	native := r.deps.EffectiveToolCallFormat(childCtx, res.Model) == "native"
	scope, _ := models.RunNetworkScopeFrom(ctx) // a child never reaches further than its parent may
	opts := assistant.ResolveRunOptions(r.deps, assistant.RunSpec{
		Kind: profile.KindSubagent, WorkspaceID: req.WorkspaceID, Model: res.Model, Profile: helper,
		Logger: r.deps.ProcessLogger(req.WorkspaceID), Observer: progress(req, res.RunID),
		Overrides: assistant.RunOverrides{AllowedTools: childTools, Memory: models.MemoryModeOff},
	})
	opts.RunNetworkScope = scope
	agent := assistant.NewAgent(client, r.deps.ToolProvider(), r.deps.Engine(), opts)
	reply, _, err := agent.Execute(childCtx, []proxy.Message{
		// The core rules and the helper's profile only: the workspace's own notes (AGENTS.md) stay on this machine.
		{Role: proxy.SystemRole, Content: prompts.AssembleSystemPrompt(prompts.PromptParts{Native: native, Profile: helper.Instructions})},
		{Role: proxy.UserRole, Content: taskPrompt(req)},
	})
	if t := usage.FromContext(childCtx); t != nil {
		res.PromptTokens, res.CompletionTokens = t.ReportedUsage()
	}
	switch {
	case err != nil:
		res.Status, res.Error = stoppedOr(ctx, assistant.SubRunFailed), err.Error()
	default:
		res.Status, res.Summary = assistant.SubRunDone, reply
	}
}

// taskPrompt is the helper's first message: the task, and the context the parent gave.
func taskPrompt(req assistant.SubRunRequest) string {
	text := fmt.Sprintf(prompts.SubagentTaskPrompt, req.Task)
	if req.Context != "" {
		text += fmt.Sprintf(prompts.SubagentContextBlock, req.Context)
	}
	return text
}

// progress turns a child's tool calls into coarse events for the parent; results, reasoning, streamed text and
// lifecycle noise stay in the child.
func progress(req assistant.SubRunRequest, runID string) assistant.Observer {
	return func(ev assistant.AgentEvent) {
		if tc, ok := ev.Payload.(proxy.ToolCall); ok && ev.Type == assistant.EventToolCall {
			emit(req, runID, assistant.SubagentToolCall, tc.Function.Name, tc.Function.Arguments)
		}
	}
}

func emit(req assistant.SubRunRequest, runID string, kind assistant.SubagentEventKind, tool, detail string) {
	if req.Emit == nil {
		return
	}
	if len(detail) > eventDetailChars {
		cut := eventDetailChars
		for cut > 0 && !utf8.RuneStart(detail[cut]) {
			cut-- // never split a character
		}
		detail = detail[:cut]
	}
	req.Emit(assistant.SubagentEvent{RunID: runID, Agent: req.Agent, Kind: kind, Tool: tool, Detail: detail})
}

// stoppedOr is stopped when the parent was cancelled, otherwise status.
func stoppedOr(ctx context.Context, status assistant.SubRunStatus) assistant.SubRunStatus {
	if ctx.Err() != nil {
		return assistant.SubRunStopped
	}
	return status
}

// closeRun releases a recording client's state for the child's run, which it keys by run id.
func closeRun(client proxy.Client, runID string) {
	if rcl, ok := client.(interface{ CloseRun(string) }); ok {
		rcl.CloseRun(runID)
	}
}
