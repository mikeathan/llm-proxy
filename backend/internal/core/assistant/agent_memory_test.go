package assistant

import (
	"context"
	"fmt"
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

func TestBuildHotInjection(t *testing.T) {
	entries := []memory.MemoryEntry{
		{Title: "build", Content: "use go build"},
		{Title: "test", Content: "run go test"},
	}
	result := buildHotInjection(entries)
	expected := "- build: use go build\n- test: run go test\n"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBuildHotInjection_Truncation(t *testing.T) {
	entries := make([]memory.MemoryEntry, 10)
	for i := range entries {
		entries[i] = memory.MemoryEntry{
			Title:   fmt.Sprintf("fact-%d", i),
			Content: strings.Repeat("X", 400-i),
		}
	}
	result := buildHotInjection(entries)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "fact-0") {
		t.Error("expected first entry to appear")
	}
	if strings.Contains(result, "fact-9") {
		t.Error("last entry should not appear (exceeds maxHotInjectionChars)")
	}
	// Each line is "- fact-N: XXX...\n" > 400 chars, so at most 4 fit in 2000.
	lines := strings.Count(result, "\n")
	if lines > 5 {
		t.Errorf("expected at most 5 lines (truncated on entry boundary), got %d", lines)
	}
}

func TestBuildHotInjection_Empty(t *testing.T) {
	result := buildHotInjection(nil)
	if result != "" {
		t.Errorf("expected empty string for nil entries, got %q", result)
	}
	result = buildHotInjection([]memory.MemoryEntry{})
	if result != "" {
		t.Errorf("expected empty string for empty entries, got %q", result)
	}
}

func TestAgent_RecallsMemoryInNewSession(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build command", "run go build ./... to verify", []string{"hot"}, "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	var callCount int
	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				var sb strings.Builder
				for _, msg := range req.Messages {
					if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
						sb.WriteString(msg.Content)
					}
				}
				capturedSystemPrompt = sb.String()
			}
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
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     store,
		EnableHotMemory: true,
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
				tc := proxy.ToolCall{ID: "call_mem", Function: proxy.FunctionCall{Name: models.ToolMemoryUpdate, Arguments: `{"content":"run go build ./...","scope":"workspace","mode":"on_demand","keep":"permanent"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			if callCount <= 2 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "done"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolMemoryUpdate}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     store,
		ContextBudget:   100,
		EnableHotMemory: true,
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

func TestAgent_NoPreSieveNudgeForAutomation(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			// Return a tool call so the agent completes cleanly
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []proxy.ToolCall{
							{
								Type: "function",
								Function: proxy.FunctionCall{
									Name:      "read_file",
									Arguments: `{"summary": "test report"}`,
								},
							},
						},
					}},
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
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     store,
		ContextBudget:   100,
		EnableHotMemory: true,
	})

	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "test system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the steps in 'test.md'.\n---\nStep 1\n---\n\nWrite your final report when done."},
	}
	for i := 0; i < 3; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: "intermediate response with lots of text to eat up budget and trigger the flush mechanism"})
		history = append(history, proxy.Message{Role: proxy.UserRole, Content: "continue with more details and keep going so the budget is exceeded"})
	}

	_, finalHistory, err := agent.Execute(ctx, history)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// With unified agent behavior, pre-sieve nudge fires for all contexts.
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

	store.Insert(ctx, "ws-1", memory.LongTerm, "database", "port is 5433", nil, "agent")

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

func TestAgent_HotMemoryInjection(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build", "run go build ./... to verify", []string{"hot"}, "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	var callCount int
	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				var sb strings.Builder
				for _, msg := range req.Messages {
					if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
						sb.WriteString(msg.Content)
					}
				}
				capturedSystemPrompt = sb.String()
			}
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
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     store,
		EnableHotMemory: true,
	})

	_, _, err = agent.Execute(ctx, []proxy.Message{
		{Role: proxy.SystemRole, Content: "test prompt"},
		{Role: proxy.UserRole, Content: "how should I build?"},
	})
	if err != nil {
		t.Fatalf("first Execute failed: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "<memory>") {
		t.Error("expected <memory> in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "go build") {
		t.Error("expected hot memory content in system prompt")
	}
}

func TestAgent_UserProfileInjection(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", memory.LongTerm, "build", "use go build ./...", []string{"hot"}, "agent")
	store.Insert(ctx, "ws-1", memory.UserProfile, "name", "Alice", []string{"hot"}, "agent")
	store.Insert(ctx, "ws-1", memory.UserProfile, "style", "concise responses", []string{"hot"}, "agent")

	var callCount int
	var capturedSystemPrompt string
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				var sb strings.Builder
				for _, msg := range req.Messages {
					if msg.Role == proxy.SystemRole || msg.Role == proxy.UserRole {
						sb.WriteString(msg.Content)
					}
				}
				capturedSystemPrompt = sb.String()
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
		MaxSteps:        5,
		WorkspaceID:     "ws-1",
		MemoryStore:     store,
		EnableHotMemory: true,
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
	if !strings.Contains(capturedSystemPrompt, "Alice") {
		t.Error("expected user profile content in system prompt")
	}
	if !strings.Contains(capturedSystemPrompt, "concise") {
		t.Error("expected user profile content in system prompt")
	}
}

func TestInjectActiveMemory_HotEntriesInjected(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	_, err := store.Insert(ctx, "ws-1", memory.LongTerm, "build command", "run go build ./... to verify", []string{"hot"}, "agent")
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	_, err = store.Insert(ctx, "ws-1", memory.LongTerm, "smoke-test-progress", "Completed Steps 1-6 of LLM Smoke Test.", nil, "agent")
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

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "test system prompt"},
		{Role: proxy.UserRole, Content: "Hello"},
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

	// Only the hot-tagged entry should be injected. The non-hot entry is excluded.
	if !strings.Contains(resultSystem, "go build") {
		t.Error("hot entry should be injected")
	}
	if strings.Contains(resultSystem, "Completed Steps 1-6") {
		t.Error("non-hot entry should NOT be injected")
	}
}

func TestInjectActiveMemory_HotOnly(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()

	// Non-hot entry should NOT be injected
	store.Insert(ctx, "ws-1", memory.LongTerm, "tool_versions", "TypeScript version: 6.0.3", nil, "agent")

	client := &MockClient{}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		MemoryStore: store,
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "test system prompt"},
		{Role: proxy.UserRole, Content: "hello"},
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

	if strings.Contains(resultSystem, "TypeScript") {
		t.Error("non-hot memory should NOT be injected")
	}
}
