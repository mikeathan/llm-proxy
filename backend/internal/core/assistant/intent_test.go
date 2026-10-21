package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
)

// A turn that ends by announcing an action it has not taken ("Let me fetch the page") is mid-task narration, not a
// final report. A real report, a past-tense account and a sign-off are not.
func TestEndsWithUnfulfilledIntent(t *testing.T) {
	yes := []string{
		"I found several significant items. Let me fetch the Alibaba pricing change and the Meta OverSeer details to confirm.",
		"Now I'll search for more details.",
		"Let me check the second source:",
		"**Next, I need to verify the date.**",
		"OK, I will run the query now.",
		"Good progress.\n\nLet me read the rest of the file.",
		"- Found two candidates\n- Let's look at the first one",
		"Great, I'm going to fetch both sources now",
		"Now let me also check the pricing page...",
	}
	for _, text := range yes {
		if !endsWithUnfulfilledIntent(text) {
			t.Errorf("not recognised as an unfulfilled intent: %q", text)
		}
	}
	no := []string{
		"",
		"Done.",
		"As of: 2026-10-21\n\n| Item | Date |\n|---|---|\n| Mercury 3 | 2026-10-20 |\n\nLet me know if you want more detail.",
		"I checked the page and found nothing new.",
		"I can see that the results show growth in open-weight releases.",
		"The vendor said: \"let me fetch it\", but the page was unchanged.",
		"| Let me fetch | the table cell | is not prose |",
		"Searches: 4. Nothing significant today.",
		"Let me know.",
		"I'll need your decision before going further.",
	}
	for _, text := range no {
		if endsWithUnfulfilledIntent(text) {
			t.Errorf("a final text was taken for an unfulfilled intent: %q", text)
		}
	}
}

// Mid-task, a text turn that only announces the next step is not accepted as the final report: the run keeps it in the
// history and nudges the model to act or to finish, at most unfulfilledIntentMax times, after which it is accepted.
func TestHandleTextTurn_UnfulfilledIntentIsNudgedNotAccepted(t *testing.T) {
	s := newTextTurnSession() // history holds a tool result: the run is mid-task
	turn := proxy.Message{Role: proxy.AssistantRole, Content: "I found several significant items. Let me fetch the pricing change to confirm."}

	done, reply, err := s.handleTextTurn(turn, nil, nil)
	if err != nil || done {
		t.Fatalf("an announced action was accepted as the final report: done=%v reply=%q err=%v", done, reply, err)
	}
	if s.recovery.intentNudges != 1 {
		t.Errorf("nudges = %d, want 1", s.recovery.intentNudges)
	}
	last := s.history[len(s.history)-1]
	if last.Role != proxy.UserRole || last.Content != prompts.UnfulfilledIntentNudge {
		t.Errorf("last history entry is not the nudge: %+v", last)
	}
	if s.history[len(s.history)-2].Content != turn.Content {
		t.Error("the model's own turn must stay in the history before the nudge")
	}

	s.recovery.intentNudges = unfulfilledIntentMax
	done, reply, err = s.handleTextTurn(turn, nil, nil)
	if err != nil || !done || !strings.Contains(reply, "Let me fetch") {
		t.Fatalf("after the bound the text must be accepted: done=%v reply=%q err=%v", done, reply, err)
	}
}

// A real report mid-task, and any text before the run has done any work, are left alone.
func TestHandleTextTurn_IntentGuardLeavesReportsAlone(t *testing.T) {
	s := newTextTurnSession()
	report := proxy.Message{Role: proxy.AssistantRole, Content: "As of: 2026-10-21\n\n| Item | Date |\n|---|---|\n| Mercury 3 | 2026-10-20 |\n\nLet me know if you need more."}
	if done, _, err := s.handleTextTurn(report, nil, nil); err != nil || !done {
		t.Fatalf("a report was held back: done=%v err=%v", done, err)
	}
	if s.recovery.intentNudges != 0 {
		t.Errorf("nudges = %d for a report", s.recovery.intentNudges)
	}
}

func TestHandleTextTurn_IntentGuardWaitsForTheRunToHaveActed(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{})
	s := newRunSession(agent, context.Background(), []proxy.Message{{Role: proxy.UserRole, Content: "what is 2+2"}})
	agent.runS = s
	turn := proxy.Message{Role: proxy.AssistantRole, Content: "I'll work it out for you now, and let me check the arithmetic."}
	if endsWithUnfulfilledIntent(turn.Content) && s.intentGuardApplies() {
		t.Error("the guard must not apply before the run has used a tool")
	}
}

// A reasoning loop cut by the repetition guard, in a run that has already used tools, is not a format problem. The agent
// does not retry it in XML mode (which drops the native tools and, in the run of 2026-10-21, ended the run on a line of
// prose); it nudges the model with the tools still offered. An empty stream and a loop on the first turn keep the retry
// chain.
func TestComputeNextResponse_LoopCutAfterToolUseKeepsNativeTools(t *testing.T) {
	native := true
	loopingStream := func() <-chan *proxy.ChatResponse {
		ch := make(chan *proxy.ChatResponse, 300)
		go func() {
			defer close(ch)
			for i := 0; i < 250; i++ {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: "I'm going to fetch both sources now. Actually, let me reconsider. "}}}}
			}
		}()
		return ch
	}
	run := func(history []proxy.Message) (nativeToolsOnRetry bool) {
		calls := 0
		client := &MockClient{
			StreamFunc: func(_ context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
				calls++
				if calls == 1 {
					return loopingStream(), nil
				}
				if calls == 2 {
					nativeToolsOnRetry = len(req.Tools) > 0
				}
				return textStream("As of today: nothing new."), nil
			},
			ChatFunc: func(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "As of today: nothing new."}}}}, nil
			},
		}
		provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}}}}
		agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, AgentOptions{MaxSteps: 5, GlobalTimeout: 5 * time.Second, UseNativeTools: &native})
		if _, _, err := agent.Execute(context.Background(), history); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return nativeToolsOnRetry
	}

	worked := []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: "{}"}}}},
		{Role: proxy.ToolRole, ToolCallID: "c1", Content: "file text"},
	}
	if !run(worked) {
		t.Error("the retry after a loop cut dropped the native tools although the run had already used tools")
	}
	first := []proxy.Message{{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"}}
	if run(first) {
		t.Error("a loop on the first turn keeps its XML retry; the retry carried native tools")
	}
}

// The nag route is tried once per stretch of progress: a second loop cut in a row falls back to the retry chain, so a
// model the nag did not stop is handled as it always was.
func TestComputeNextResponse_SecondLoopCutFallsBackToTheRetryChain(t *testing.T) {
	native := true
	loop := func() <-chan *proxy.ChatResponse {
		ch := make(chan *proxy.ChatResponse, 300)
		go func() {
			defer close(ch)
			for i := 0; i < 250; i++ {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: "I'm going to fetch both sources now. Actually, let me reconsider. "}}}}
			}
		}()
		return ch
	}
	var toolsPerCall []int
	client := &MockClient{
		StreamFunc: func(_ context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			toolsPerCall = append(toolsPerCall, len(req.Tools))
			if len(toolsPerCall) <= 2 {
				return loop(), nil
			}
			return textStream("As of today: nothing new."), nil
		},
		ChatFunc: func(_ context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			toolsPerCall = append(toolsPerCall, len(req.Tools))
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "As of today: nothing new."}}}}, nil
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}}}}
	agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, AgentOptions{MaxSteps: 6, GlobalTimeout: 10 * time.Second, UseNativeTools: &native})
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: "{}"}}}},
		{Role: proxy.ToolRole, ToolCallID: "c1", Content: "file text"},
	}
	if _, _, err := agent.Execute(context.Background(), history); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(toolsPerCall) < 3 || toolsPerCall[1] == 0 {
		t.Fatalf("the first loop cut must be retried with the native tools; tools per call %v", toolsPerCall)
	}
	if toolsPerCall[2] != 0 {
		t.Errorf("the second loop cut must use the retry chain (no native tools); tools per call %v", toolsPerCall)
	}
}
