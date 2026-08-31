// usagetracker.go — Per-execution usage tracker embedded in context.
// Counts LLM calls and tool invocations for observability.
package assistant

import (
	"context"
	"sync"
	"time"
)

type usageKeyType struct{}

var usageKey usageKeyType

type UsageTracker struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	LLMCalls        int
	ToolCalls       int
	UsedTools       []string
	ExecutionTime   time.Duration
	mu              sync.Mutex
}

func WithUsageTracker(ctx context.Context) context.Context {
	if GetUsageTracker(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, usageKey, &UsageTracker{})
}

func GetUsageTracker(ctx context.Context) *UsageTracker {
	if t, ok := ctx.Value(usageKey).(*UsageTracker); ok {
		return t
	}
	return nil
}

func (t *UsageTracker) AddLLMCall(inputTokens, outputTokens, reasoningTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LLMCalls++
	t.InputTokens += inputTokens
	t.OutputTokens += outputTokens
	t.ReasoningTokens += reasoningTokens
}

func (t *UsageTracker) AddToolCall(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ToolCalls++
	t.UsedTools = append(t.UsedTools, name)
}

// UsedToolsSnapshot returns a copy of the tool names used this run. Used by
// the synthesized run summary — the per-execution record survives sieving,
// whereas scanning the run's (pruned) history under-counts tool activity.
func (t *UsageTracker) UsedToolsSnapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.UsedTools))
	copy(out, t.UsedTools)
	return out
}
