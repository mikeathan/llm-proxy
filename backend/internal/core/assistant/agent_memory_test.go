package assistant

import (
	"context"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

func newTestMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "agent-memory-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.DB().Close() })

	memStore, err := memory.New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return memStore
}

func TestAgent_RecallsMemoryInNewSession(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build command", "run go build ./... to verify", "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			var sb strings.Builder
			for _, msg := range req.Messages {
				if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
					sb.WriteString(msg.Content)
				}
			}
			capturedSystemPrompt = sb.String()
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nTask finished"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	_, _, err = agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "how should I build?"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "<memory>") {
		t.Error("expected <memory> block in messages")
	}
	if !strings.Contains(capturedSystemPrompt, "go build") {
		t.Error("expected seeded memory content in messages")
	}
}

func TestAgent_NoMemoryStore_NoInjection(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			for _, msg := range req.Messages {
				if msg.Role == proxy.SystemRole && strings.Contains(msg.Content, "<memory>") {
					t.Error("should not inject memory when no store is set")
				}
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nOK"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: nil,
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestAgent_WritesMemoryBeforeSieve(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	callCount := 0
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount <= 1 {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{
						{
							Message: proxy.Message{
								Role: "assistant",
								ToolCalls: []proxy.ToolCall{
									{
										ID: "call_mem",
										Function: proxy.FunctionCall{
											Name:      models.ToolMemoryUpdate,
											Arguments: `{"topic":"build","content":"run go build ./...","memory_type":"long_term"}`,
										},
									},
								},
							},
						},
					},
				}, nil
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nTask finished"}},
				},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolMemoryUpdate}},
		},
	}
	engine := &MockEngine{Result: map[string]any{"status": "saved"}}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		WorkspaceID:   "ws-1",
		MemoryStore:   store,
		ContextBudget: 100,
	})

	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "test system prompt that is very long to eat up budget"},
		{Role: proxy.UserRole, Content: "please do something"},
	}
	for i := 0; i < 3; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: "intermediate response with lots of text to fill up the context budget and trigger the memory flush mechanism"})
		history = append(history, proxy.Message{Role: proxy.UserRole, Content: "continue please with more details and keep going so the budget is exceeded"})
	}

	reply, finalHistory, err := agent.Execute(ctx, history)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if reply == "" {
		t.Error("expected non-empty reply")
	}

	// Verify the nudge was injected into history at some point
	nudgeFound := false
	for _, msg := range finalHistory {
		if msg.Role == proxy.UserRole && strings.Contains(msg.Content, "memory_update") {
			nudgeFound = true
			break
		}
	}
	if !nudgeFound {
		t.Error("expected pre-sieve memory nudge in history")
	}
}

func TestAgent_ActiveMemory_NoMatch_NoInjection(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", memory.LongTerm, "database", "port is 5433", "agent")

	captured := ""
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			for _, msg := range req.Messages {
				if msg.Role == proxy.SystemRole {
					captured = msg.Content
					break
				}
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nOK"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	_, _, err := agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "xyzzy unicorn magic"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if strings.Contains(captured, "<memory>") {
		t.Error("should not inject memory when query doesn't match")
	}
}

func TestAgent_UsageMeterInPrompt(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build", "run go build ./... to verify", "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			var sb strings.Builder
			for _, msg := range req.Messages {
				if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
					sb.WriteString(msg.Content)
				}
			}
			capturedSystemPrompt = sb.String()
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nTask finished"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	_, _, err = agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "how should I build?"},
	})

	_, _, err = agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "build"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "<memory>") {
		t.Error("expected <memory> in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "[memory store:") {
		t.Error("expected usage meter in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "/4000 chars]") {
		t.Error("expected char limit indicator in usage meter")
	}
}

func TestAgent_UserProfileInjection(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", memory.LongTerm, "build", "use go build ./...", "agent")
	store.Insert(ctx, "ws-1", memory.UserProfile, "name", "Alice", "agent")
	store.Insert(ctx, "ws-1", memory.UserProfile, "style", "concise responses", "agent")

	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			var sb strings.Builder
			for _, msg := range req.Messages {
				if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
					sb.WriteString(msg.Content)
				}
			}
			capturedSystemPrompt = sb.String()
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "# Done\nOK"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	_, _, err := agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "build preferences"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "<memory>") {
		t.Error("expected <memory> in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "<user_profile>") {
		t.Error("expected <user_profile> in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "Alice") {
		t.Error("expected user profile content in system prompt")
	}
}

func TestInjectActiveMemory_KeepsProgressInAutomation(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build command", "run go build ./... to verify", "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	_, err = store.Insert(ctx, "ws-1", memory.LongTerm, "llm-smoke-test-progress", "Completed Steps 1-6 of LLM Smoke Test.", "agent")
	if err != nil {
		t.Fatalf("seed progress memory: %v", err)
	}

	client := &MockClient{}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	automationMsg := prompts.AutomationMarker + " in workspace 'ws-1'.\n" +
		"Run go build and smoke-test steps.\n" +
		"Execute the instructions found in 'test.md':\n---\nStep 1\n---\n"
	systemPrompt := "test system prompt"
	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: systemPrompt},
		{Role: proxy.UserRole, Content: automationMsg},
	}

	result := agent.injectActiveMemory(prepared, prepared)

	var resultSystem string
	var sb strings.Builder
	for _, msg := range result {
		if msg.Role == proxy.SystemRole {
			sb.WriteString(msg.Content)
		}
	}
	resultSystem = sb.String()

	// Both memories should be injected — the task prompt guidance tells the agent
	// how to use the progress memory, not filter it.
	if !strings.Contains(resultSystem, "Completed Steps 1-6") {
		t.Error("progress memory should still be injected in automation context")
	}
	if !strings.Contains(resultSystem, "go build") {
		t.Error("non-progress memory should still be injected")
	}
}

func TestInjectActiveMemory_CachesTaskPrompt(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "tool_versions", "TypeScript version: 6.0.3", "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	client := &MockClient{}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	if agent.automationTaskPrompt != "" {
		t.Fatal("automationTaskPrompt should start empty")
	}

	taskPrompt := prompts.AutomationMarker + " in workspace 'ws-1'.\n" +
		"Run go build and typescript steps.\n" +
		"Execute the instructions found in 'test.md':\n---\nStep 1\n---\n"
	systemPrompt := "test system prompt"

	// First call: history contains the full task prompt
	firstPrepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: systemPrompt},
		{Role: proxy.UserRole, Content: taskPrompt},
	}
	result1 := agent.injectActiveMemory(firstPrepared, firstPrepared)
	if agent.automationTaskPrompt != taskPrompt {
		t.Fatal("agent should cache the task prompt after first invocation")
	}

	var result1System string
	{
		var sb strings.Builder
		for _, msg := range result1 {
			if msg.Role == proxy.SystemRole {
				sb.WriteString(msg.Content)
			}
		}
		result1System = sb.String()
	}
	if !strings.Contains(result1System, "TypeScript") {
		t.Error("memory should be injected when the full task prompt is present")
	}

	// Second call: history contains only tool results — no task prompt
	secondPrepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: systemPrompt},
		{Role: proxy.UserRole, Content: "Observation: file written successfully"},
	}
	result2 := agent.injectActiveMemory(secondPrepared, secondPrepared)

	var result2System string
	{
		var sb strings.Builder
		for _, msg := range result2 {
			if msg.Role == proxy.SystemRole {
				sb.WriteString(msg.Content)
			}
		}
		result2System = sb.String()
	}
	if !strings.Contains(result2System, "TypeScript") {
		t.Error("memory should still be injected using cached task prompt despite degraded lastUserMsg")
	}
}
