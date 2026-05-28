package assistant

import (
	"context"
	"sync"
	"testing"
)

func TestUsageTracker_AddLLMCall(t *testing.T) {
	tracker := &UsageTracker{}
	tracker.AddLLMCall(100, 50, 10)

	if tracker.LLMCalls != 1 {
		t.Errorf("expected 1 LLM call, got %d", tracker.LLMCalls)
	}
	if tracker.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", tracker.InputTokens)
	}
	if tracker.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", tracker.OutputTokens)
	}
	if tracker.ReasoningTokens != 10 {
		t.Errorf("expected 10 reasoning tokens, got %d", tracker.ReasoningTokens)
	}
}

func TestUsageTracker_AddToolCall(t *testing.T) {
	tracker := &UsageTracker{}
	tracker.AddToolCall("read_file")
	tracker.AddToolCall("write_file")

	if tracker.ToolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", tracker.ToolCalls)
	}
	if len(tracker.UsedTools) != 2 {
		t.Errorf("expected 2 used tools, got %d", len(tracker.UsedTools))
	}
	if tracker.UsedTools[0] != "read_file" {
		t.Errorf("expected first tool 'read_file', got '%s'", tracker.UsedTools[0])
	}
	if tracker.UsedTools[1] != "write_file" {
		t.Errorf("expected second tool 'write_file', got '%s'", tracker.UsedTools[1])
	}
}

func TestUsageTracker_Concurrency(t *testing.T) {
	tracker := &UsageTracker{}
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.AddLLMCall(10, 5, 1)
			tracker.AddToolCall("test_tool")
		}()
	}
	wg.Wait()

	if tracker.LLMCalls != 100 {
		t.Errorf("expected 100 LLM calls, got %d", tracker.LLMCalls)
	}
	if tracker.ToolCalls != 100 {
		t.Errorf("expected 100 tool calls, got %d", tracker.ToolCalls)
	}
	if tracker.InputTokens != 1000 {
		t.Errorf("expected 1000 input tokens, got %d", tracker.InputTokens)
	}
}

func TestGetUsageTracker_NoTracker(t *testing.T) {
	ctx := context.Background()
	tracker := GetUsageTracker(ctx)
	if tracker != nil {
		t.Error("expected nil tracker when context has no tracker")
	}
}

func TestGetUsageTracker_WithTracker(t *testing.T) {
	ctx := withUsageTracker(context.Background())
	tracker := GetUsageTracker(ctx)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.LLMCalls != 0 {
		t.Errorf("expected 0 calls on fresh tracker, got %d", tracker.LLMCalls)
	}
}

func TestUsageTracker_CumulativeMetrics(t *testing.T) {
	tracker := &UsageTracker{}
	tracker.AddLLMCall(50, 20, 5)
	tracker.AddLLMCall(30, 10, 2)

	if tracker.LLMCalls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", tracker.LLMCalls)
	}
	if tracker.InputTokens != 80 {
		t.Errorf("expected 80 total input tokens, got %d", tracker.InputTokens)
	}
	if tracker.OutputTokens != 30 {
		t.Errorf("expected 30 total output tokens, got %d", tracker.OutputTokens)
	}
	if tracker.ReasoningTokens != 7 {
		t.Errorf("expected 7 total reasoning tokens, got %d", tracker.ReasoningTokens)
	}
}
