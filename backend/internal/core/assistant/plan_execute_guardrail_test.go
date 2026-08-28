package assistant

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// TestExecutePlan_GuardrailDeniedStepContinues verifies a guardrail-denied plan
// step does not abort the remaining plan: the denial is recorded as a tool
// result and the next step still executes (mirrors processToolCalls).
func TestExecutePlan_GuardrailDeniedStepContinues(t *testing.T) {
	// Terminal is statically disabled → any execute_terminal_command step is
	// denied at guardrail time (deterministic, no OnGuardrail callback).
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Terminal: models.TerminalGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolTerminalExecute}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
	}}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(&MockClient{}, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		Guardrails:  gr,
	})

	plan := &ExecutionPlan{
		Description: "guardrail-resilient plan",
		Steps: []ExecutionStep{
			{ToolName: models.ToolTerminalExecute, Description: "denied step", Parameters: map[string]any{"command": "rm -rf /"}},
			{ToolName: "test_tool", Description: "allowed step", Parameters: map[string]any{"key": "val"}},
		},
	}

	reply, history, err := agent.executePlan(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	}, plan)
	if err != nil {
		t.Fatalf("executePlan failed: %v", err)
	}
	if reply != "[Plan execution complete]" {
		t.Errorf("expected plan completion marker, got %q", reply)
	}
	if engine.Calls != 1 {
		t.Errorf("expected remaining plan step to execute (1 tool call), got %d", engine.Calls)
	}

	denialRecorded := false
	stepRan := false
	for _, m := range history {
		if m.Role == proxy.ToolRole {
			if strings.Contains(m.Content, "terminal tools are disabled") {
				denialRecorded = true
			}
			if strings.Contains(m.Content, "ok") {
				stepRan = true
			}
		}
	}
	if !denialRecorded {
		t.Error("expected the guardrail denial to be recorded as a tool result")
	}
	if !stepRan {
		t.Error("expected the remaining plan step's tool result in history")
	}
}

// failOnceEngine is a MockEngine-compatible engine whose first call fails like
// a shell command exiting non-zero; subsequent calls succeed. Used to drive
// plan-step execution failures deterministically.
type failOnceEngine struct {
	calls int
}

func (e *failOnceEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	e.calls++
	if e.calls == 1 {
		return nil, fmt.Errorf("shell execution failed: exit status 2")
	}
	return "ok", nil
}

// TestExecutePlan_StepExecutionErrorContinues verifies a plan step whose tool
// execution fails (e.g. a shell command exiting non-zero) does NOT abort the
// remaining plan: the error is recorded as a tool result and the next step
// still executes. Regression: the llm-smoke-test run aborted on a `tsc` step
// that exited 2, discarding all prior successful steps and the report.
func TestExecutePlan_StepExecutionErrorContinues(t *testing.T) {
	engine := &failOnceEngine{}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
	}}
	agent := NewAgent(&MockClient{}, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
	})

	plan := &ExecutionPlan{
		Description: "resilient plan",
		Steps: []ExecutionStep{
			{ToolName: "test_tool", Description: "failing step", Parameters: map[string]any{"key": "val"}},
			{ToolName: "test_tool", Description: "next step", Parameters: map[string]any{"key": "val2"}},
		},
	}

	reply, history, err := agent.executePlan(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	}, plan)
	if err != nil {
		t.Fatalf("executePlan must not abort on a failed step: %v", err)
	}
	if reply != "[Plan execution complete]" {
		t.Errorf("expected plan completion marker, got %q", reply)
	}
	if engine.calls != 2 {
		t.Errorf("expected both plan steps to execute (2 tool calls), got %d", engine.calls)
	}

	// The failing step's error must be visible in history so the finalization
	// turn (and the final report) can note it.
	failureRecorded := false
	for _, m := range history {
		if m.Role == proxy.ToolRole && strings.Contains(m.Content, "exit status 2") {
			failureRecorded = true
		}
	}
	if !failureRecorded {
		t.Error("expected the failing step's error to be recorded as a tool result in history")
	}
}

// TestExecutePlan_StepExecutionErrorRecorded verifies the failure text lands in
// the tool-result history even when every step fails (an all-fail plan still
// completes so the report can say so).
func TestExecutePlan_StepExecutionErrorRecorded(t *testing.T) {
	engine := &MockEngine{Err: fmt.Errorf("shell execution failed: exit status 2")}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
	}}
	agent := NewAgent(&MockClient{}, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
	})

	plan := &ExecutionPlan{
		Description: "all-fail plan",
		Steps: []ExecutionStep{
			{ToolName: "test_tool", Description: "step 1", Parameters: map[string]any{"key": "val"}},
			{ToolName: "test_tool", Description: "step 2", Parameters: map[string]any{"key": "val2"}},
		},
	}

	_, history, err := agent.executePlan(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
	}, plan)
	if err != nil {
		t.Fatalf("executePlan must not abort on failed steps: %v", err)
	}
	if engine.Calls != 2 {
		t.Errorf("expected both steps to execute, got %d", engine.Calls)
	}
	failures := 0
	for _, m := range history {
		if m.Role == proxy.ToolRole && strings.Contains(m.Content, "exit status 2") {
			failures++
		}
	}
	if failures != 2 {
		t.Errorf("expected 2 recorded step failures in history, got %d", failures)
	}
}
