package automation

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

func TestBuildPrompt_IncludesMemoryCheckGate(t *testing.T) {
	taskContent := "Step 1: list directory\nStep 2: run npx tsc --version"
	req := ExecuteRequest{
		WorkspaceID:    "test-ws",
		AutomationName: "test-automation",
		TaskFile:       "test-task.md",
		TaskContent:    taskContent,
	}

	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	result := executor.buildPrompt(taskContent, req)

	if !strings.Contains(result, "[Memory Check Gate]") {
		t.Errorf("expected buildPrompt output to contain MemoryCheckGate, got:\n%s", result)
	}
	if !strings.Contains(result, taskContent) {
		t.Errorf("expected buildPrompt output to contain original task content, got:\n%s", result)
	}
	if !strings.Contains(result, prompts.AutomationMarker) {
		t.Errorf("expected buildPrompt output to contain AutomationMarker, got:\n%s", result)
	}
}

func TestBuildAssistantPrefill_WithEntries(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	entries := []memory.MemoryEntry{
		{Title: "tool_versions", Content: "TypeScript version installed: 6.0.3"},
		{Title: "system_os_info", Content: "OS detected: Darwin 25.5.0"},
	}

	result := executor.buildAssistantPrefill(entries)

	if result == "" {
		t.Fatal("buildAssistantPrefill returned empty for non-empty entries")
	}
	if !strings.Contains(result, "tool_versions") {
		t.Errorf("expected prefill to contain memory title 'tool_versions', got:\n%s", result)
	}
	if !strings.Contains(result, "TypeScript version installed: 6.0.3") {
		t.Errorf("expected prefill to contain memory content, got:\n%s", result)
	}
	if !strings.Contains(result, "skip that step") {
		t.Errorf("expected prefill to contain skip instruction, got:\n%s", result)
	}
}

func TestBuildAssistantPrefill_EmptyEntries(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	result := executor.buildAssistantPrefill(nil)
	if result != "" {
		t.Errorf("expected empty prefill for nil entries, got:\n%s", result)
	}

	result = executor.buildAssistantPrefill([]memory.MemoryEntry{})
	if result != "" {
		t.Errorf("expected empty prefill for empty entries, got:\n%s", result)
	}
}

func TestAnnotateTaskWithMemories_AddsAnnotations(t *testing.T) {
	store := newTestMemoryStore(t)
	_, err := store.Insert(context.Background(), "ws-1", memory.LongTerm,
		"tool_versions", "TypeScript version installed: 6.0.3", "agent")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	taskContent := "Step 5: install TypeScript and run npx tsc --version"

	svc := &mockSvc{memoryStore: store}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	result, count := executor.annotateTaskWithMemories(context.Background(), "ws-1", taskContent)

	if count == 0 {
		t.Fatal("expected annotations for task with matching memory")
	}
	if !strings.Contains(result, "↳ [Memory:") {
		t.Errorf("expected annotation marker in result, got:\n%s", result)
	}
	if !strings.Contains(result, "tool_versions") {
		t.Errorf("expected memory title in annotation, got:\n%s", result)
	}
}

func TestAnnotateTaskWithMemories_NilStore(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	_, count := executor.annotateTaskWithMemories(context.Background(), "ws-1", "Step 3: run uname -a")

	if count != 0 {
		t.Errorf("expected 0 annotations with nil store, got %d", count)
	}
}

func TestAnnotateTaskWithMemories_NoWordOverlapSkips(t *testing.T) {
	store := newTestMemoryStore(t)
	_, err := store.Insert(context.Background(), "ws-1", memory.LongTerm,
		"compliance_audit", "all checks passed and verified", "agent")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	// No non-stop words overlap between "fetch data from httpbin.org"
	// and "compliance_audit: all checks passed and verified".
	taskContent := "Step 9: fetch data from httpbin.org"

	svc := &mockSvc{memoryStore: store}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	result, count := executor.annotateTaskWithMemories(context.Background(), "ws-1", taskContent)

	if count != 0 {
		t.Errorf("expected 0 annotations when no word overlap, got %d", count)
	}
	if result != taskContent {
		t.Errorf("expected unchanged task content, got:\n%s", result)
	}
}

func TestAnnotateTaskWithMemories_MaxFive(t *testing.T) {
	store := newTestMemoryStore(t)
	// Insert one memory that will match every line.
	_, err := store.Insert(context.Background(), "ws-1", memory.LongTerm,
		"common_test", "file directory list common", "agent")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	// 10 identical lines — each matches the memory with word overlap.
	lines := strings.Repeat("Step X: list directory files\n", 10)
	taskContent := strings.TrimSpace(lines)

	svc := &mockSvc{memoryStore: store}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	result, count := executor.annotateTaskWithMemories(context.Background(), "ws-1", taskContent)

	if count > 5 {
		t.Errorf("expected at most 5 annotations, got %d", count)
	}
	if !strings.Contains(result, "↳ [Memory:") {
		t.Errorf("expected annotations in result, got:\n%s", result)
	}
}

func TestBuildPriorRunSeededHistory_NoPriorRun(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	req := ExecuteRequest{AutomationName: "smoke-test"}
	result := executor.buildPriorRunSeededHistory(req)

	if result != nil {
		t.Errorf("expected nil for no prior run, got %d messages", len(result))
	}
}

func TestBuildPriorRunSeededHistory_EmptyPriorRun(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	req := ExecuteRequest{
		AutomationName: "smoke-test",
		State: &models.AgentState{
			LastRuns: map[string]*models.AutomationRun{
				"smoke-test": {Error: ""},
			},
		},
	}
	result := executor.buildPriorRunSeededHistory(req)

	if len(result) != 0 {
		t.Errorf("expected empty for run with no events, got %d messages", len(result))
	}
}

func TestBuildPriorRunSeededHistory_SeedsEvenWithError(t *testing.T) {
	svc := &mockSvc{}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	req := ExecuteRequest{
		AutomationName: "smoke-test",
		State: &models.AgentState{
			LastRuns: map[string]*models.AutomationRun{
				"smoke-test": {
					Error: "something went wrong",
					Events: []any{
						assistant.AgentEvent{
							Type: assistant.EventToolCall,
							Payload: proxy.ToolCall{
								ID: "call_1", Type: "function",
								Function: proxy.FunctionCall{Name: "list_directory", Arguments: `{}`},
							},
						},
					},
				},
			},
		},
	}
	result := executor.buildPriorRunSeededHistory(req)

	if len(result) == 0 {
		t.Fatal("expected seeded messages even when prior run had an error")
	}
	if result[0].Role != proxy.AssistantRole {
		t.Errorf("expected assistant role for tool call, got %s", result[0].Role)
	}
}

func newTestMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-test-*.db")
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

// mockSvc implements a minimal LLMServiceProvider for testing.
type mockSvc struct {
	memoryStore *memory.Store
}

func (m *mockSvc) ClientProvider() proxy.LLMClientProvider                     { return nil }
func (m *mockSvc) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockSvc) ModelConfig(modelName string) (models.ModelConfig, bool)     { return models.ModelConfig{}, false }
func (m *mockSvc) Logger() logging.Logger                                      { return nil }
func (m *mockSvc) ToolProvider() assistant.ToolProvider                         { return nil }
func (m *mockSvc) Engine() assistant.Engine                                     { return nil }
func (m *mockSvc) GuardrailEngine() *guardrails.GuardrailEngine                 { return nil }
func (m *mockSvc) GuardrailDecisionStore() *assistant.GuardrailDecisionStore     { return nil }
func (m *mockSvc) ProcessLogger(workspaceID string) logging.Logger              { return nil }
func (m *mockSvc) Persistence() *persistence.WorkspaceManager                    { return nil }
func (m *mockSvc) Events() *EventBus                                            { return nil }
func (m *mockSvc) Orchestrator() *orchestrator.Orchestrator                     { return nil }
func (m *mockSvc) MemoryStore() *memory.Store                                   { return m.memoryStore }
func (m *mockSvc) GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockSvc) RecordDir() string { return "" }
func (m *mockSvc) RootDir() string  { return "" }
func (m *mockSvc) RunLoggingEnabled() bool { return false }
func (m *mockSvc) SelectModels() (string, string) { return "", "" }
