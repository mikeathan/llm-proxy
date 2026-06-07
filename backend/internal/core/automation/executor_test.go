package automation

import (
	"context"
	"database/sql"
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

func TestBuildPrompt_IncludesTaskContent(t *testing.T) {
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

	if !strings.Contains(result, taskContent) {
		t.Errorf("expected buildPrompt output to contain original task content, got:\n%s", result)
	}
	if !strings.Contains(result, prompts.AutomationMarker) {
		t.Errorf("expected buildPrompt output to contain AutomationMarker, got:\n%s", result)
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

func (m *mockSvc) ClientProvider() proxy.LLMClientProvider { return nil }
func (m *mockSvc) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockSvc) ModelConfig(modelName string) (models.ModelConfig, bool) {
	return models.ModelConfig{}, false
}
func (m *mockSvc) Logger() logging.Logger                                    { return nil }
func (m *mockSvc) ToolProvider() assistant.ToolProvider                      { return nil }
func (m *mockSvc) Engine() assistant.Engine                                  { return nil }
func (m *mockSvc) GuardrailEngine() *guardrails.GuardrailEngine              { return nil }
func (m *mockSvc) GuardrailDecisionStore() *assistant.GuardrailDecisionStore { return nil }
func (m *mockSvc) ProcessLogger(workspaceID string) logging.Logger           { return nil }
func (m *mockSvc) Persistence() *persistence.WorkspaceManager                { return nil }
func (m *mockSvc) Events() *EventBus                                         { return nil }
func (m *mockSvc) Orchestrator() *orchestrator.Orchestrator                  { return nil }
func (m *mockSvc) MemoryStore() *memory.Store                                { return m.memoryStore }
func (m *mockSvc) GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockSvc) RecordDir() string              { return "" }
func (m *mockSvc) RootDir() string                { return "" }
func (m *mockSvc) RunLoggingEnabled() bool        { return false }
func (m *mockSvc) SelectModels() (string, string) { return "", "" }

func newTestMemoryStoreAndDB(t *testing.T) (*memory.Store, *sql.DB) {
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
	database := p.DB()
	t.Cleanup(func() { database.Close() })

	memStore, err := memory.New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return memStore, database
}
