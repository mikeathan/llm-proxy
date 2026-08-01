package assistant

import (
	"context"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
)

// streamFromChunks returns a StreamFunc that emits the given chunks then closes.
func streamFromChunks(chunks []*proxy.ChatResponse) func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return func(_ context.Context, _ proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		ch := make(chan *proxy.ChatResponse, len(chunks))
		for _, c := range chunks {
			ch <- c
		}
		close(ch)
		return ch, nil
	}
}

func TestAgentThinkingEmitted_OpaqueProvider(t *testing.T) {
	// Opaque model: stream returns content only, NO reasoning_content. The UI
	// must still receive a neutral "working" signal before the first message.
	client := &MockClient{
		StreamFunc: streamFromChunks([]*proxy.ChatResponse{
			{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Hello"}}}},
			{Choices: []proxy.Choice{{Delta: proxy.Message{Content: " world"}}}},
			{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "Hello world"}}}},
		}),
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:          3,
		MaxResponseTokens: 2048,
		ProviderType:      "openai",
	})
	var events []AgentEvent
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " say hi"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	foundThinking := false
	for _, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseAgentThinking {
				foundThinking = true
				break
			}
		}
	}
	if !foundThinking {
		t.Fatal("expected agent_thinking lifecycle event for opaque provider, got none")
	}

	// agent_thinking must appear before the first assistant-content message
	// event (the model's actual reply). User/system messages may precede it.
	thinkIdx := -1
	for i, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseAgentThinking {
				thinkIdx = i
				break
			}
		}
	}
	if thinkIdx < 0 {
		t.Fatal("agent_thinking event not found")
	}
	for _, ev := range events[:thinkIdx] {
		if ev.Type == EventMessage {
			if m, ok := ev.Payload.(proxy.Message); ok && m.Role == proxy.AssistantRole && m.Content != "" {
				t.Error("an assistant-content message preceded agent_thinking; working signal was not first")
			}
		}
		if ev.Type == EventReasoning {
			t.Error("a reasoning event preceded agent_thinking")
		}
	}
}

func TestAgentThinkingEmitted_ReasoningProvider(t *testing.T) {
	// Reasoning model: stream returns reasoning_content. The neutral signal must
	// still fire (and precede the reasoning/content deltas).
	client := &MockClient{
		StreamFunc: streamFromChunks([]*proxy.ChatResponse{
			{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: "let me think"}}}},
			{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "answer"}}}},
			{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "answer", ReasoningContent: "let me think"}}}},
		}),
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:          3,
		MaxResponseTokens: 2048,
		ProviderType:      "nvidia",
	})
	var events []AgentEvent
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " solve it"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	foundThinking := false
	for _, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseAgentThinking {
				foundThinking = true
				break
			}
		}
	}
	if !foundThinking {
		t.Fatal("expected agent_thinking lifecycle event for reasoning provider, got none")
	}
}

func TestAgentThinkingPayloadHasNoReasoningContent(t *testing.T) {
	// Safety: the working signal must carry NO reasoning text that could be
	// mistaken for model output / surface as reasoning-panel spam.
	client := &MockClient{
		StreamFunc: streamFromChunks([]*proxy.ChatResponse{
			{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "done"}}}},
		}),
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps: 2, MaxResponseTokens: 1024, ProviderType: "openai",
	})
	var events []AgentEvent
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }
	_, _, _ = agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " go"},
	})
	for _, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseAgentThinking {
				for _, banned := range []string{"reasoning", "content", "text"} {
					if _, ok := p[banned]; ok {
						t.Errorf("agent_thinking must not carry %q field", banned)
					}
				}
			}
		}
	}
}
