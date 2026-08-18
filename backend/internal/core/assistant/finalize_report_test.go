package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

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
