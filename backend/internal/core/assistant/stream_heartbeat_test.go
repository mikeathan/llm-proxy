package assistant

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

// setStreamHeartbeatInterval overrides the package-level heartbeat cadence for
// the duration of a test. Assistant tests are not parallel, so a temporary
// package-global override is safe.
func setStreamHeartbeatInterval(t *testing.T, interval time.Duration) {
	t.Helper()
	old := streamHeartbeatInterval
	streamHeartbeatInterval = interval
	t.Cleanup(func() { streamHeartbeatInterval = old })
}

func setNonStreamHeartbeatInterval(t *testing.T, interval time.Duration) {
	t.Helper()
	old := nonStreamHeartbeatInterval
	nonStreamHeartbeatInterval = interval
	t.Cleanup(func() { nonStreamHeartbeatInterval = old })
}

// lifecyclePhases returns the ordered lifecycle phase strings seen by the
// observer, plus a count of still_thinking events.
type lifeEvents struct {
	mu            sync.Mutex
	phases        []string
	stillThinking []map[string]any
}

func (l *lifeEvents) observer() Observer {
	return func(ev AgentEvent) {
		if ev.Type != EventLifecycle {
			return
		}
		p, ok := ev.Payload.(map[string]any)
		if !ok {
			return
		}
		phase, _ := p["phase"].(string)
		l.mu.Lock()
		defer l.mu.Unlock()
		l.phases = append(l.phases, phase)
		if phase == PhaseStillThinking {
			l.stillThinking = append(l.stillThinking, p)
		}
	}
}

func (l *lifeEvents) count(phase string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, p := range l.phases {
		if p == phase {
			n++
		}
	}
	return n
}

// TestStreamHeartbeat_EmitsStillThinkingOnSilentStall: a stream that emits
// content then goes silent for longer than one heartbeat cadence must emit
// still_thinking so the UI bubble never looks dead.
func TestStreamHeartbeat_EmitsStillThinkingOnSilentStall(t *testing.T) {
	setStreamHeartbeatInterval(t, 5*time.Millisecond)

	events := &lifeEvents{}
	agent := &Agent{
		config: AgentConfig{Channel: "test", MaxTokens: 1_000_000},
		deps: AgentRuntimeDeps{
			Logger:   logging.NewNopLogger(),
			Observer: events.observer(),
		},
	}

	feed := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(feed)
		feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "start"}}}}
		// Stall longer than several heartbeat ticks.
		time.Sleep(60 * time.Millisecond)
		feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: " finished"}}}}
	}()

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), feed, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}

	if events.count(PhaseStillThinking) == 0 {
		t.Fatal("expected still_thinking during silent stall, got none")
	}
	if !strings.Contains(fullMsg.Content, "start") || !strings.Contains(fullMsg.Content, "finished") {
		t.Errorf("expected both content chunks, got %q", fullMsg.Content)
	}
}

// TestStreamHeartbeat_NoStillThinkingDuringActiveStreaming: a continuously
// advancing stream must NOT emit still_thinking (silent-stall gate keeps the
// bus quiet while the model is actively producing output).
func TestStreamHeartbeat_NoStillThinkingDuringActiveStreaming(t *testing.T) {
	// Chunks arrive faster than the heartbeat cadence so content always
	// advances between ticks — the silent-stall gate must stay quiet.
	setStreamHeartbeatInterval(t, 5*time.Millisecond)

	events := &lifeEvents{}
	agent := &Agent{
		config: AgentConfig{Channel: "test", MaxTokens: 1_000_000},
		deps: AgentRuntimeDeps{
			Logger:   logging.NewNopLogger(),
			Observer: events.observer(),
		},
	}

	feed := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(feed)
		for i := 0; i < 100; i++ {
			feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "x"}}}}
			time.Sleep(time.Millisecond)
		}
	}()

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), feed, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}

	if n := events.count(PhaseStillThinking); n != 0 {
		t.Fatalf("expected no still_thinking during active streaming, got %d", n)
	}
}

// TestStreamHeartbeat_StillThinkingContentFree: still_thinking must carry no
// content field, mirroring the agent_thinking contract.
func TestStreamHeartbeat_StillThinkingContentFree(t *testing.T) {
	setStreamHeartbeatInterval(t, 2*time.Millisecond)

	events := &lifeEvents{}
	agent := &Agent{
		config: AgentConfig{Channel: "test", MaxTokens: 1_000_000},
		deps: AgentRuntimeDeps{
			Logger:   logging.NewNopLogger(),
			Observer: events.observer(),
		},
	}

	feed := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(feed)
		feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "start"}}}}
		time.Sleep(40 * time.Millisecond)
		feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: " end"}}}}
	}()

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), feed, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}

	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.stillThinking) == 0 {
		t.Fatal("expected still_thinking events")
	}
	for _, p := range events.stillThinking {
		for _, banned := range []string{"content", "reasoning", "text"} {
			if _, ok := p[banned]; ok {
				t.Errorf("still_thinking must not carry %q field", banned)
			}
		}
		if _, ok := p["elapsed"]; !ok {
			t.Error("still_thinking must carry elapsed field")
		}
	}
}

// TestNonStreamHeartbeat_EmitsFallbackWaiting: the non-stream path (fallback
// after streaming is unavailable) must still emit fallback_waiting via the
// Heartbeat while it waits on a slow provider response.
func TestNonStreamHeartbeat_EmitsFallbackWaiting(t *testing.T) {
	setNonStreamHeartbeatInterval(t, 5*time.Millisecond)

	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(60 * time.Millisecond):
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "done"}}},
				}, nil
			}
		},
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:          2,
		MaxResponseTokens: 1024,
		ProviderType:      "openai",
	})
	events := &lifeEvents{}
	agent.deps.Observer = events.observer()

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " go"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if events.count("fallback_waiting") == 0 {
		t.Fatal("expected fallback_waiting lifecycle event from non-stream heartbeat, got none")
	}
}
