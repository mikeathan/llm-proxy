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
	modelCfg    models.ModelConfig
}

func (m *mockSvc) ClientProvider() proxy.LLMClientProvider { return nil }
func (m *mockSvc) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockSvc) ModelConfig(modelName string) (models.ModelConfig, bool) {
	if m.modelCfg.Name == modelName && m.modelCfg.Name != "" {
		return m.modelCfg, true
	}
	return models.ModelConfig{}, false
}
func (m *mockSvc) EffectiveToolCallFormat(ctx context.Context, modelName string) string { return "" }
func (m *mockSvc) Logger() logging.Logger                                               { return nil }
func (m *mockSvc) ToolProvider() assistant.ToolProvider                                 { return nil }
func (m *mockSvc) Engine() assistant.Engine                                             { return nil }
func (m *mockSvc) GuardrailEngine() *guardrails.GuardrailEngine                         { return nil }
func (m *mockSvc) GuardrailDecisionStore() *assistant.GuardrailDecisionStore            { return nil }
func (m *mockSvc) ProcessLogger(workspaceID string) logging.Logger                      { return nil }
func (m *mockSvc) Persistence() *persistence.WorkspaceManager                           { return nil }
func (m *mockSvc) Events() assistant.EventPublisher                                     { return nil }
func (m *mockSvc) Orchestrator() *orchestrator.Orchestrator                             { return nil }
func (m *mockSvc) MemoryStore() *memory.Store                                           { return m.memoryStore }
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

// TestBuildAgentOptions_NativeToolFormatPropagates verifies the ordering
// contract that fixes the cold-cache race (2026-08-31 14:26 run): after
// EffectiveToolCallFormat persists "native" onto the stored model config,
// buildAgentOptions → ApplyModelConfig must lock UseNativeTools=true so the
// agent is built with native tool calling. If the probe is resolved AFTER the
// agent build, the first run post-restart silently runs in XML text mode.
func TestBuildAgentOptions_NativeToolFormatPropagates(t *testing.T) {
	cfg := models.ModelConfig{
		Name:            "Ornith-1.5-35B-Q4_K_M.gguf",
		ToolCallFormat:  "native", // post-probe persisted state
		MaxSteps:        10,
		MaxTokens:       2048,
		ReasoningBudget: 512,
	}
	svc := &mockSvc{modelCfg: cfg}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	req := ExecuteRequest{
		WorkspaceID:    "test-ws",
		AutomationName: "smoke-test",
		TaskFile:       "task.md",
		TaskContent:    "do the thing",
		Model:          cfg.Name,
	}
	opts := executor.buildAgentOptions(req, logging.NewNopLogger(), new([]any), nil)
	if opts.UseNativeTools == nil || !*opts.UseNativeTools {
		t.Fatalf("expected UseNativeTools=true after native format resolution, got %v", opts.UseNativeTools)
	}
	if opts.ModelName != cfg.Name {
		t.Errorf("expected ModelName %q, got %q", cfg.Name, opts.ModelName)
	}
}

// TestBuildAgentOptions_ColdCacheDefaultsXML verifies the pre-fix failure
// shape: a stored config with NO tool_call_format (probe not yet run) must NOT
// accidentally enable native — the agent defaults to XML text mode, which the
// executor's resolve-before-build ordering then corrects by probing first.
func TestBuildAgentOptions_ColdCacheDefaultsXML(t *testing.T) {
	cfg := models.ModelConfig{
		Name:      "cold-model",
		MaxSteps:  10,
		MaxTokens: 2048,
	}
	svc := &mockSvc{modelCfg: cfg}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)

	req := ExecuteRequest{WorkspaceID: "test-ws", Model: cfg.Name, TaskContent: "x"}
	opts := executor.buildAgentOptions(req, logging.NewNopLogger(), new([]any), nil)
	if opts.UseNativeTools != nil && *opts.UseNativeTools {
		t.Fatalf("expected UseNativeTools unset/false with no format in config, got %v", opts.UseNativeTools)
	}
}
