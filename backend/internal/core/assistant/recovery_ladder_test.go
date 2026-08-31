package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// streamMsg returns a StreamFunc-compatible channel emitting a single message.
// Tool calls / content are placed in Delta (the real streaming shape that
// processStream consumes) rather than Message.
func streamMsg(msg proxy.Message) func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		ch := make(chan *proxy.ChatResponse, 1)
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: msg}}}
		close(ch)
		return ch, nil
	}
}

// TestRecoveryLadder_FinalizationDisablesTools verifies that the deterministic
// finalization turn runs with Tools=nil and ToolChoice=none, forcing the model
// to deliver a text report regardless of provider/tool support.
func TestRecoveryLadder_FinalizationDisablesTools(t *testing.T) {
	var finalizeReq proxy.ChatRequest
	var sawFinalize bool
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			callCount++
			// Calls 1-3: model returns an empty turn (reasoning + no tool calls)
			// so the ladder marches nudge→nudge→finalize.
			if req.ToolChoice == proxy.ToolChoiceNone {
				sawFinalize = true
				finalizeReq = req
				return streamMsg(proxy.Message{Role: "assistant", Content: "## Dev Report\nCompilation succeeded, all tests passed."})(ctx, req)
			}
			return streamMsg(proxy.Message{Role: "assistant", Content: " ", ReasoningContent: "thinking about next step"})(ctx, req)
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "execute_terminal_command"}},
	}}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       10,
		UseNativeTools: boolPtr(true),
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " build the project"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !sawFinalize {
		t.Fatal("finalization turn (ToolChoice=none) was never issued")
	}
	if len(finalizeReq.Tools) != 0 {
		t.Errorf("finalization turn must disable tools, got %d tools", len(finalizeReq.Tools))
	}
	if finalizeReq.ToolChoice != proxy.ToolChoiceNone {
		t.Errorf("finalization turn must set ToolChoice=none, got %q", finalizeReq.ToolChoice)
	}
	if !strings.Contains(reply, "Compilation succeeded") {
		t.Errorf("expected final report text, got %q", reply)
	}
}

// TestRecoveryLadder_BoundedNoInfiniteLoop verifies that repeated empty turns
// after tool work are bounded: exactly postToolNudgeMax nudges, then one
// finalization turn, then a terminal exit — never an infinite loop.
func TestRecoveryLadder_BoundedNoInfiniteLoop(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			callCount++
			// Always an empty turn (reasoning present, no tool calls).
			return streamMsg(proxy.Message{Role: "assistant", Content: " ", ReasoningContent: "thinking"})(ctx, req)
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "execute_terminal_command"}},
	}}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       50,
		UseNativeTools: boolPtr(true),
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do work"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 2 nudges + 1 finalization turn + the finalization turn's empty result
	// (which hits the terminal step) = bounded. Must be far below MaxSteps.
	if callCount > postToolNudgeMax+3 {
		t.Errorf("expected bounded calls (~%d), got %d", postToolNudgeMax+3, callCount)
	}
	_ = reply
}

// TestRecoveryLadder_DevHappyPath verifies the real-world dev scenario: the
// model runs a tool, then returns an empty turn, gets re-nudged, and finally
// emits its report as chat text (not a file dump).
func TestRecoveryLadder_DevHappyPath(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// Run a tool (e.g. write the source file).
				return streamMsg(proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{{
					ID:   "c1",
					Type: "function",
					Function: proxy.FunctionCall{
						Name:      models.ToolFileWrite,
						Arguments: `{"path":"app.ts","content":"console.log(1)"}`,
					},
				}}})(ctx, req)
			case 2:
				// Empty turn after the tool result (model is "thinking").
				return streamMsg(proxy.Message{Role: "assistant", Content: " ", ReasoningContent: "thinking"})(ctx, req)
			default:
				// Finalization / later turn: the real report as chat text.
				return streamMsg(proxy.Message{Role: "assistant", Content: "## Source\napp.ts compiles cleanly.\n\n## Compilation\n0 errors.\n\n## Output\n1"})(ctx, req)
			}
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolFileWrite}},
	}}
	engine := &MockEngine{Result: "written"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       10,
		UseNativeTools: boolPtr(true),
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " write and report"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(reply, "compiles cleanly") {
		t.Errorf("expected dev chat report, got %q", reply)
	}
}

// TestRecoveryLadder_NetworkTruncatedWriteSalvaged verifies the network-scan
// deliverable: when the model streams a long report into a write_file call whose
// arguments are truncated (invalid JSON), the report is recovered at EXECUTION
// TIME (salvageTruncatedWrite in processToolCalls) and returned as the reply —
// without ever reaching the no-tool recovery ladder or dumping a source file.
func TestRecoveryLadder_NetworkTruncatedWriteSalvaged(t *testing.T) {
	callCount := 0
	long := strings.Repeat("r", salvageMinContentLen) + " network scan report body"
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			callCount++
			// Truncated write_file: valid JSON up to the content value, then cut off.
			return streamMsg(proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{{
				ID:   "c1",
				Type: "function",
				Function: proxy.FunctionCall{
					Name:      models.ToolFileWrite,
					Arguments: `{"path":"final-report.md","content":"` + long,
				},
			}}})(ctx, req)
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{
			Name:       models.ToolFileWrite,
			Parameters: map[string]any{"required": []string{"path", "content"}},
		}},
	}}
	engine := &MockEngine{Result: "written"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       10,
		UseNativeTools: boolPtr(true),
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " scan the network and write the report"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(reply, "network scan report body") {
		t.Errorf("expected salvaged truncated write as reply, got %q", reply)
	}
	if callCount != 1 {
		t.Errorf("execution-time salvage should complete on the write turn, got %d calls", callCount)
	}
}

// TestRecoveryLadder_ToolErrorThenTruncatedThought_NotCompleted is the
// regression test for the llm-smoke-test failure: the terminal tool returns an
// error, the model replies with a truncated ReAct scaffold ("Thought: ...
// Action:" with no tool call), and the loop must NOT record that incomplete
// turn as the final answer. It must reject the dangling Action delimiter (both
// in checkTaskCompletion and in the parse-error trust branch) and give the
// model feedback so it can continue, instead of finalizing an incomplete run
// as "completed".
func TestRecoveryLadder_ToolErrorThenTruncatedThought_NotCompleted(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// Model runs the terminal tool (XML text mode — the mode the
				// failing smoke test ran in) and the tool FAILS.
				return streamMsg(proxy.Message{
					Role: "assistant",
					Content: "Thought: Step 1 done. Now running Step 3 system commands.\n\n<tool_call>\n" +
						`{"tool": "execute_terminal_command", "args": {"command": "uname -a\ndate -u +%Y-%m-%dT%H:%M:%SZ\necho \"terminal-tool-works\""}}` +
						"\n</tool_call>",
				})(ctx, req)
			case 2:
				// Truncated retry: Thought + dangling Action marker, no tool call.
				return streamMsg(proxy.Message{
					Role:    "assistant",
					Content: "Thought: `uname -a` isn't supported on this system. I'll report this as-is. Now running the date and echo commands (Step 3).\n\nAction:",
				})(ctx, req)
			default:
				// After feedback the model delivers a real report.
				return streamMsg(proxy.Message{
					Role:    "assistant",
					Content: "# Final Report\nStep 3 completed: all three system commands produced output.",
				})(ctx, req)
			}
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "execute_terminal_command"}},
	}}
	// The engine fails the terminal call AND returns raw output — the exact
	// shape that carries no JSON error field in the recorded tool result.
	engine := &MockEngine{
		Result: "usage: uname [-amnoprsv]\n",
		Err:    errors.New("shell execution failed: exit status 1"),
	}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       10,
		UseNativeTools: boolPtr(false),
		// Mirrors the failing model's runtime config: the openai provider tier
		// has Prefill=false, so no <tool_call>{"tool":" prefill is injected and
		// the model's XML output arrives clean.
		ProviderType: "openai",
	})
	reply, history, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " run the smoke test steps"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if strings.Contains(reply, "uname -a` isn't supported") {
		t.Fatalf("truncated retry turn was recorded as the final answer: %q", reply)
	}
	if !strings.Contains(reply, "Final Report") {
		t.Fatalf("expected the real final report, got %q", reply)
	}
	// The truncated scaffold is part of the transcript (the model said it) but
	// must be followed by recovery feedback, not treated as the final answer.
	_ = history
}
