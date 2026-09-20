// Package usage tracks per-execution LLM and tool usage, embedded in the run
// context for observability.
package usage

import (
	"context"
	"sync"
	"time"
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
	mu              sync.Mutex
}

// WithTracker installs a fresh tracker on ctx unless one is already present.
func WithTracker(ctx context.Context) context.Context {
	if FromContext(ctx) != nil {
		return ctx
	}
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
