package usage

import (
	"context"
	"sync"
	"testing"

	"llm-proxy/models"
)

func TestTracker_AddLLMCall(t *testing.T) {
	tracker := &Tracker{}
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

func TestTracker_AddToolCall(t *testing.T) {
	tracker := &Tracker{}
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

func TestTracker_Concurrency(t *testing.T) {
	tracker := &Tracker{}
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

func TestFromContext_NoTracker(t *testing.T) {
	ctx := context.Background()
	tracker := FromContext(ctx)
	if tracker != nil {
		t.Error("expected nil tracker when context has no tracker")
	}
}

func TestFromContext_WithTracker(t *testing.T) {
	ctx := WithTracker(context.Background())
	tracker := FromContext(ctx)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.LLMCalls != 0 {
		t.Errorf("expected 0 calls on fresh tracker, got %d", tracker.LLMCalls)
	}
}

func TestTracker_CumulativeMetrics(t *testing.T) {
	tracker := &Tracker{}
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

func TestTracker_ReportedCachedTokens(t *testing.T) {
	tr := &Tracker{}
	tr.AddReportedUsage(100, 10)
	tr.AddCachedTokens(60)
	tr.AddCachedTokens(0)
	tr.AddCachedTokens(25)
	if got := tr.ReportedCached(); got != 85 {
		t.Errorf("ReportedCached() = %d, want 85", got)
	}
	if p, c := tr.ReportedUsage(); p != 100 || c != 10 {
		t.Errorf("ReportedUsage() = %d/%d, want 100/10: the cache count must not change them", p, c)
	}
}

// A sub-agent counts into a tracker of its own; the parent keeps only a record of each delegated task.
func TestTracker_SubagentsAndAFreshTrackerForAChild(t *testing.T) {
	parentCtx := WithTracker(context.Background())
	parent := FromContext(parentCtx)
	child := FromContext(WithNewTracker(parentCtx))
	if child == nil || child == parent {
		t.Fatal("a child run gets a tracker of its own")
	}
	child.AddReportedUsage(500, 50)
	if p, c := parent.ReportedUsage(); p != 0 || c != 0 {
		t.Errorf("the child's tokens reached the parent: %d, %d", p, c)
	}
	parent.AddSubagent(models.SubagentRun{Agent: "web-researcher", Status: "done", PromptTokens: 500})
	got := parent.Subagents()
	if len(got) != 1 || got[0].PromptTokens != 500 {
		t.Errorf("subagents = %+v", got)
	}
	got[0].Agent = "changed"
	if parent.Subagents()[0].Agent != "web-researcher" {
		t.Error("Subagents returns a copy")
	}
}

func TestTracker_ContextOverflows(t *testing.T) {
	tr := &Tracker{}
	tr.AddContextOverflow()
	tr.AddContextOverflow()
	if got := tr.ContextOverflows(); got != 2 {
		t.Errorf("ContextOverflows() = %d, want 2", got)
	}
}
