package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
)

// finalizeMockClient builds a MockClient whose finalization turn (the only LLM
// call finalizeReport makes) returns the given report via the non-streaming
// fallback. The request is captured for ToolChoice/Tools assertions.
func finalizeMockClient(report string, seen *proxy.ChatRequest) *MockClient {
	return &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			if seen != nil {
				*seen = req
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    proxy.AssistantRole,
				Content: report,
			}}}}, nil
		},
	}
}

// TestFinalizeReport_ToolsDisabledTurn verifies the finalization turn runs with
// Tools=nil and ToolChoice=none (forced text report) regardless of provider
// tool support.
func TestFinalizeReport_ToolsDisabledTurn(t *testing.T) {
	var seen proxy.ChatRequest
	client := finalizeMockClient("# Report\nFinal answer.", &seen)
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, UseNativeTools: boolPtr(true)})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	})
	agent.runS = s

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}
	if report != "# Report\nFinal answer." {
		t.Errorf("expected report text, got %q", report)
	}
	if len(seen.Tools) != 0 {
		t.Errorf("finalization turn must disable tools, got %d tools", len(seen.Tools))
	}
	if seen.ToolChoice != proxy.ToolChoiceNone {
		t.Errorf("finalization turn must set ToolChoice=none, got %q", seen.ToolChoice)
	}
}

// TestFinalizeReport_EmptyOutputFallsBack verifies an empty finalization turn
// falls back to the best available assistant answer instead of returning a
// blank report.
func TestFinalizeReport_EmptyOutputFallsBack(t *testing.T) {
	client := finalizeMockClient("   ", nil)
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
		{Role: proxy.AssistantRole, Content: "Partial result from earlier tool work."},
		{Role: proxy.ToolRole, Content: "data"},
	})
	agent.runS = s

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}
	if report != "Partial result from earlier tool work." {
		t.Errorf("expected bestAvailableAnswer fallback, got %q", report)
	}
}

// TestFinalizeReport_BareActionMarkerFallsBack verifies the finalization turn
// rejects a truncated ReAct scaffold (content ending on a bare "Action:" line)
// and falls back to the best available answer instead of emitting the scaffold
// as the final report — the same guard checkTaskCompletion applies, kept
// uniform across all loop strategies (plan-execute and the react ladder both
// seal through finalizeReport).
func TestFinalizeReport_BareActionMarkerFallsBack(t *testing.T) {
	client := finalizeMockClient("Thought: one more step to verify.\n\nAction:", nil)
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
		{Role: proxy.AssistantRole, Content: "Real partial result from earlier tool work."},
		{Role: proxy.ToolRole, Content: "data"},
	})
	agent.runS = s

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}
	if strings.Contains(report, "Action:") {
		t.Fatalf("truncated scaffold must not become the final report, got %q", report)
	}
	if report != "Real partial result from earlier tool work." {
		t.Errorf("expected bestAvailableAnswer fallback, got %q", report)
	}
}

// TestFinalizeReport_EmitsEventMessage verifies the report is emitted as an
// EventMessage so the frontend renders it (and finalizes) without relying on a
// follow-up lifecycle signal.
func TestFinalizeReport_EmitsEventMessage(t *testing.T) {
	var events []AgentEvent
	client := finalizeMockClient("# Report\nFinal answer.", nil)
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps: 5,
		Observer: func(ev AgentEvent) { events = append(events, ev) },
	})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	})
	agent.runS = s

	if _, err := s.finalizeReport(context.Background()); err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}

	found := false
	for _, ev := range events {
		if ev.Type == EventMessage {
			if msg, ok := ev.Payload.(proxy.Message); ok && msg.Role == proxy.AssistantRole && msg.Content == "# Report\nFinal answer." {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected an EventMessage carrying the report")
	}
}

// TestFinalizeReport_LLMFailure_SynthesizesSummary verifies that when the
// finalization LLM turn fails and no assistant text exists to salvage (the
// plan_execute shape: history is pure tool calls), the run still completes
// with a degraded-but-real summary synthesized from the tool activity —
// a provider outage on the report turn must not discard completed work.
// Regression: the llm-smoke-test run executed all 11 plan steps but died with
// "finalization turn failed" when NVIDIA timed out on the report call.
func TestFinalizeReport_LLMFailure_SynthesizesSummary(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, errors.New("upstream timeout failure after 3 attempts")
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{"key":"val"}`}}}},
		{Role: proxy.ToolRole, ToolCallID: "c1", Content: `{"error":"shell execution failed: exit status 2"}`},
	})
	agent.runS = s

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport must not fail when work was done: %v", err)
	}
	if !strings.Contains(report, "test_tool") {
		t.Errorf("synthesized summary must mention the executed tool, got %q", report)
	}
	if !strings.Contains(report, "exit status 2") {
		t.Errorf("synthesized summary must surface the recorded failure, got %q", report)
	}
	if strings.TrimSpace(report) == "" {
		t.Error("synthesized summary must not be empty")
	}
}

// TestFinalizeReport_LLMFailure_NoWorkStillFails verifies the fail-loud path
// is preserved: when the finalization LLM turn fails AND the run did no tool
// work (nothing to synthesize), finalizeReport still returns an error.
func TestFinalizeReport_LLMFailure_NoWorkStillFails(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, errors.New("upstream timeout failure after 3 attempts")
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	})
	agent.runS = s

	if _, err := s.finalizeReport(context.Background()); err == nil {
		t.Fatal("expected finalizeReport to fail when no work exists to summarize")
	}
}

func TestBestAvailableAnswer_SkipsToolCallMarkup(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "list files"},
		// A truncated <tool_call> attempt streamed as visible content (the
		// upstream-outage shape) must never be salvaged as the answer.
		{Role: proxy.AssistantRole, Content: "<tool_call>\n{\"tool\": \"list_directory\",\n  \"args\": {\n    \"path\": \".\"\n  }\n</tool_call>"},
		{Role: proxy.AssistantRole, Content: "The workspace contains: AGENTS.md, dev-test/."},
	})
	if got := s.bestAvailableAnswer(); got != "The workspace contains: AGENTS.md, dev-test/." {
		t.Fatalf("expected the real answer, got %q", got)
	}
}

func TestBestAvailableAnswer_OnlyMarkupReturnsEmpty(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "list files"},
		{Role: proxy.AssistantRole, Content: "<tool_call>{\"tool\": \"list_directory\"}\n</tool_call>"},
	})
	if got := s.bestAvailableAnswer(); got != "" {
		t.Fatalf("expected empty when the only content is tool-call markup, got %q", got)
	}
}

// newFinalizeTruncationSession builds a MockClient that serves the finalization
// turn via the non-streaming fallback, delegating each call's response to resp
// (called with the 1-based call index), plus the agent/session wired to it.
// The returned counter records how many LLM calls the client served. Shared by
// the finalization length-continuation tests.
func newFinalizeTruncationSession(t *testing.T, resp func(call int) proxy.Choice) (*runSession, *int) {
	t.Helper()
	calls := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			calls++
			return &proxy.ChatResponse{Choices: []proxy.Choice{resp(calls)}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}
	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, UseNativeTools: boolPtr(true)})
	s := newRunSession(agent, context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
		{Role: proxy.ToolRole, Content: "ok"},
	})
	agent.runS = s
	return s, &calls
}

// TestFinalizeReport_LengthTruncatedContinuation verifies the finalization
// turn — when it hits the output-token cap mid-report (finish_reason="length")
// — is nudged once to continue and the fragments are stitched into the report.
func TestFinalizeReport_LengthTruncatedContinuation(t *testing.T) {
	s, calls := newFinalizeTruncationSession(t, func(call int) proxy.Choice {
		if call == 1 {
			// First finalization turn truncated by the output cap.
			return proxy.Choice{
				Message:      proxy.Message{Role: proxy.AssistantRole, Content: "# Report\n## 1. Filesystem\nlisted"},
				FinishReason: "length",
			}
		}
		// Continuation completes the report.
		return proxy.Choice{
			Message:      proxy.Message{Role: proxy.AssistantRole, Content: "files and verified the write succeeded."},
			FinishReason: "stop",
		}
	})

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}
	if *calls != 2 {
		t.Errorf("expected 2 LLM calls (truncated + continuation), got %d", *calls)
	}
	want := "# Report\n## 1. Filesystem\nlisted\nfiles and verified the write succeeded."
	if report != want {
		t.Errorf("expected stitched report %q, got %q", want, report)
	}
	if s.lengthContinuationCount != 1 {
		t.Errorf("expected 1 continuation, got %d", s.lengthContinuationCount)
	}
}

// TestFinalizeReport_LengthContinuationBounded verifies the finalization
// continuation respects lengthContinuationMax — a finalization turn that keeps
// truncating cannot exceed the bound.
func TestFinalizeReport_LengthContinuationBounded(t *testing.T) {
	s, calls := newFinalizeTruncationSession(t, func(call int) proxy.Choice {
		return proxy.Choice{
			Message:      proxy.Message{Role: proxy.AssistantRole, Content: "still truncated fragment of the report."},
			FinishReason: "length",
		}
	})

	report, err := s.finalizeReport(context.Background())
	if err != nil {
		t.Fatalf("finalizeReport failed: %v", err)
	}
	// 1 initial + lengthContinuationMax continuations, then the partial is kept.
	if *calls != 1+lengthContinuationMax {
		t.Errorf("expected %d calls (bounded), got %d", 1+lengthContinuationMax, *calls)
	}
	if !strings.Contains(report, "still truncated") {
		t.Errorf("expected the partial fragments in the report, got %q", report)
	}
	if s.lengthContinuationCount != lengthContinuationMax {
		t.Errorf("expected continuation count at the bound, got %d", s.lengthContinuationCount)
	}
}

// TestIsAgentControlMessage_LengthContinuationPrompt verifies the continuation
// nudge is registered as a synthetic control message so completion detection
// never mistakes it for real user text.
func TestIsAgentControlMessage_LengthContinuationPrompt(t *testing.T) {
	if !isAgentControlMessage(proxy.Message{Role: proxy.UserRole, Content: prompts.LengthContinuationPrompt}) {
		t.Error("LengthContinuationPrompt must be recognized as a control message")
	}
}
