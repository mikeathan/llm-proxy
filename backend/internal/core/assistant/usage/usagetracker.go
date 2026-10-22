// Package usage tracks per-execution LLM and tool usage, embedded in the run
// context for observability.
package usage

import (
	"context"
	"slices"
	"sync"
	"time"

	"llm-proxy/models"
)

type usageKeyType struct{}

var usageKey usageKeyType

type Tracker struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	LLMCalls        int
	ToolCalls       int
	UsedTools       []string
	ExecutionTime   time.Duration
	// Token counts the provider itself reported (TokenUsage), summed over the
	// run. InputTokens/OutputTokens above are local size counts, not tokens.
	reportedPrompt     int
	reportedCompletion int
	reportedCached     int
	subagents          []models.SubagentRun
	mu                 sync.Mutex
}

// WithTracker installs a fresh tracker on ctx unless one is already present.
func WithTracker(ctx context.Context) context.Context {
	if FromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, usageKey, &Tracker{})
}

// WithNewTracker installs a fresh tracker on ctx even when one is present: a sub-agent's context derives from its
// parent's, and its tokens must not count as the parent's.
func WithNewTracker(ctx context.Context) context.Context {
	return context.WithValue(ctx, usageKey, &Tracker{})
}

// FromContext returns the run's tracker, or nil when none was installed.
func FromContext(ctx context.Context) *Tracker {
	if t, ok := ctx.Value(usageKey).(*Tracker); ok {
		return t
	}
	return nil
}

func (t *Tracker) AddLLMCall(inputTokens, outputTokens, reasoningTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LLMCalls++
	t.InputTokens += inputTokens
	t.OutputTokens += outputTokens
	t.ReasoningTokens += reasoningTokens
}

// AddReportedUsage adds one LLM call's provider-reported token counts.
func (t *Tracker) AddReportedUsage(promptTokens, completionTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reportedPrompt += promptTokens
	t.reportedCompletion += completionTokens
}

// AddCachedTokens adds how many of one LLM call's prompt tokens the server's prompt cache already held.
func (t *Tracker) AddCachedTokens(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reportedCached += n
}

// ReportedCached returns the cached prompt tokens summed over the run (0 when the provider reported none).
func (t *Tracker) ReportedCached() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reportedCached
}

// ReportedUsage returns the provider-reported prompt and completion tokens
// summed over the run (0, 0 when the provider reported none).
func (t *Tracker) ReportedUsage() (promptTokens, completionTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reportedPrompt, t.reportedCompletion
}

func (t *Tracker) AddToolCall(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ToolCalls++
	t.UsedTools = append(t.UsedTools, name)
}

// UsedToolsSnapshot returns a copy of the tool names used this run. Used by
// the synthesized run summary — the per-execution record survives sieving,
// whereas scanning the run's (pruned) history under-counts tool activity.
func (t *Tracker) UsedToolsSnapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.UsedTools))
	copy(out, t.UsedTools)
	return out
}

// AddSubagent records one delegated task of the run.
func (t *Tracker) AddSubagent(run models.SubagentRun) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subagents = append(t.subagents, run)
}

// Subagents returns a copy of the run's delegated-task records, nil when it delegated nothing.
func (t *Tracker) Subagents() []models.SubagentRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.subagents)
}
