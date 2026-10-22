// delegate.go — the delegate tool. The agent owns it because everything it needs is the parent run's: the helpers its
// profile lists, its event stream, its result cap and its run record. Each task runs through the SubRunner port as a
// child run; the parent reads back one short summary per task, never the child's history.
package assistant

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

const (
	maxDelegateTasks   = 3   // tasks in one call; they run at the same time
	maxSubagentsPerRun = 6   // tasks across one run
	minSummaryChars    = 600 // a summary is never cut below this, whatever the parent's cap
	taskLabelChars     = 80  // how much of a task names its part of the result
)

// delegateToolSchema is the delegate tool's schema, read once from its embedded manifest.
var delegateToolSchema = sync.OnceValues(func() (proxy.Tool, error) {
	params, desc, err := tools.LoadManifestAsTool(models.CategoryDelegation, models.ToolDelegate)
	if err != nil {
		return proxy.Tool{}, fmt.Errorf("load the delegate tool manifest: %w", err)
	}
	return proxy.Tool{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolDelegate, Description: desc, Parameters: params}}, nil
})

// delegatingProvider adds the delegate tool to a run's tools while delegation is set up. Whether a run may use it is
// its profile's business: a profile that names no helpers hides it, like any tool it does not offer.
type delegatingProvider struct {
	ToolProvider
	runner SubRunner
}

func withDelegateTool(provider ToolProvider, runner SubRunner) ToolProvider {
	if runner == nil {
		return provider
	}
	return delegatingProvider{ToolProvider: provider, runner: runner}
}

func (p delegatingProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	list, err := p.ToolProvider.ListTools(ctx)
	if err != nil || !p.runner.Enabled() {
		return list, err
	}
	tool, err := delegateToolSchema()
	if err != nil {
		return list, nil // a broken manifest is a build defect the manifest tests catch; the run goes on without it
	}
	return append(slices.Clone(list), tool), nil
}

type delegateArgs struct {
	Tasks []delegateTask `json:"tasks"`
}

type delegateTask struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Context string `json:"context"`
}

// runDelegate runs the call's tasks at once and returns their summaries as plain text. A malformed call is an error
// the model can correct; a task that could not run (busy, refused, failed) is part of the result, never an error, so
// one busy helper cannot end the parent's run.
func (a *Agent) runDelegate(ctx context.Context, tc proxy.ToolCall) (any, error) {
	tasks, err := a.delegateTasks(tc.Function.Arguments)
	if err != nil {
		return nil, err
	}
	a.subagentsUsed += len(tasks)

	results := make([]SubRunResult, len(tasks))
	emit := a.subagentEmitter()
	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Go(func() {
			results[i] = a.deps.SubRunner.Run(ctx, SubRunRequest{
				WorkspaceID: a.config.WorkspaceID, Agent: task.Agent, Task: task.Task, Context: task.Context, Emit: emit,
			})
		})
	}
	wg.Wait()

	recordSubagents(ctx, tasks, results)
	return a.delegateResult(tasks, results), nil
}

// delegateTasks checks a call: 1..maxDelegateTasks tasks, each with a question, each naming a helper the run's profile
// lists, and the run's total within maxSubagentsPerRun.
func (a *Agent) delegateTasks(rawArgs string) ([]delegateTask, error) {
	var args delegateArgs
	if err := proxy.DecodeToolArgs(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("delegate_task arguments: %w", err)
	}
	if n := len(args.Tasks); n == 0 || n > maxDelegateTasks {
		return nil, fmt.Errorf(prompts.SubagentErrNoTasks, maxDelegateTasks, n)
	}
	if a.subagentsUsed+len(args.Tasks) > maxSubagentsPerRun {
		return nil, fmt.Errorf(prompts.SubagentErrRunLimit, a.subagentsUsed)
	}
	helpers := a.config.Profile.Delegates
	for i, task := range args.Tasks {
		if strings.TrimSpace(task.Task) == "" {
			return nil, fmt.Errorf(prompts.SubagentErrEmptyTask, i+1)
		}
		if !slices.Contains(helpers, task.Agent) {
			return nil, fmt.Errorf(prompts.SubagentErrUnknownHelper, task.Agent, strings.Join(helpers, ", "))
		}
	}
	return args.Tasks, nil
}

// subagentEmitter forwards children's progress into the parent's stream as nested events. Children run at once and
// the parent's observer is not safe for concurrent use, so calls are serialised.
func (a *Agent) subagentEmitter() func(SubagentEvent) {
	var mu sync.Mutex
	return func(ev SubagentEvent) {
		mu.Lock()
		defer mu.Unlock()
		a.notify(EventSubagent, ev)
	}
}

// recordSubagents adds each task's record to the parent run's tracker (the turn's run record reads it).
func recordSubagents(ctx context.Context, tasks []delegateTask, results []SubRunResult) {
	tracker := usage.FromContext(ctx)
	if tracker == nil {
		return
	}
	for i, res := range results {
		tracker.AddSubagent(models.SubagentRun{
			Agent: tasks[i].Agent, Model: res.Model, Status: string(res.Status), DurationMs: res.DurationMs,
			PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
		})
	}
}

// delegateResult is the text the parent reads: one part per task, each cut to a share of the parent's result cap so
// the whole fits without the parent's own truncation cutting a summary in the middle.
func (a *Agent) delegateResult(tasks []delegateTask, results []SubRunResult) string {
	share := max(minSummaryChars, a.currentResultCap()/len(tasks))
	var sb strings.Builder
	for i, res := range results {
		header := fmt.Sprintf(prompts.SubagentResultHeader, tasks[i].Agent, cutAtBoundary(tasks[i].Task, taskLabelChars))
		sb.WriteString(header)
		sb.WriteString(cutAtBoundary(subagentText(res), share-len(header)))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// subagentText is a task's answer, or a note saying why there is none.
func subagentText(res SubRunResult) string {
	switch res.Status {
	case SubRunDone:
		if strings.TrimSpace(res.Summary) == "" {
			return prompts.SubagentNoAnswer
		}
		return strings.TrimSpace(res.Summary)
	case SubRunBusy:
		return prompts.SubagentBusy
	case SubRunRefused:
		return fmt.Sprintf(prompts.SubagentRefused, res.Error)
	case SubRunStopped:
		return prompts.SubagentStopped
	default:
		return fmt.Sprintf(prompts.SubagentFailed, res.Error)
	}
}

// cutAtBoundary keeps text within limit characters, cut at the last line or sentence end when one is near, and says
// so when it cut.
func cutAtBoundary(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	keep := max(0, limit-len(prompts.SubagentSummaryCut))
	cut := text[:keep]
	if i := strings.LastIndexAny(cut, "\n."); i > keep/2 {
		cut = cut[:i+1]
	}
	return strings.TrimSpace(cut) + prompts.SubagentSummaryCut
}
