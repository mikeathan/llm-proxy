package assistant

import (
	"context"
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
