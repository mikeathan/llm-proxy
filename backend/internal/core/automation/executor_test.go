package automation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
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

// stubClient is a minimal proxy.Client for waitForModelReady tests.
type stubClient struct{}

func (stubClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return nil, nil
}
func (stubClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return nil, nil
}
func (stubClient) ReasoningField() string { return proxy.DefaultReasoningField }

// TestWaitForModelReady_PollsUntilReady verifies that a cold local model start
// (ErrModelStarting on the first calls) is polled rather than failing the run,
// and that the poll stops as soon as a client is available.
func TestWaitForModelReady_PollsUntilReady(t *testing.T) {
	calls := 0
	get := func() (proxy.Client, error) {
		calls++
		if calls < 3 {
			return nil, models.ErrModelStarting
		}
		return stubClient{}, nil
	}

	client, err := waitForModelReady(context.Background(), get, "local-gguf", time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("expected ready client, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if calls != 3 {
		t.Errorf("expected 3 poll attempts (2 starting + 1 ready), got %d", calls)
	}
}

// TestWaitForModelReady_FailsFastOnNonStartingError verifies that an error
// other than ErrModelStarting aborts immediately without polling.
func TestWaitForModelReady_FailsFastOnNonStartingError(t *testing.T) {
	calls := 0
	get := func() (proxy.Client, error) {
		calls++
		return nil, fmt.Errorf("boom")
	}
	if _, err := waitForModelReady(context.Background(), get, "m", time.Millisecond, time.Second); err == nil {
		t.Fatal("expected hard error, got nil")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 attempt, got %d", calls)
	}
}

// TestWaitForModelReady_TimesOut verifies the poll gives up with a clear error
// when the model never becomes ready within the wait budget.
func TestWaitForModelReady_TimesOut(t *testing.T) {
	get := func() (proxy.Client, error) {
		return nil, models.ErrModelStarting
	}
	_, err := waitForModelReady(context.Background(), get, "never-ready", 2*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("expected 'did not become ready' in error, got: %v", err)
	}
}

// TestWaitForModelReady_CancelledContext verifies a cancelled caller context
// aborts the poll immediately.
func TestWaitForModelReady_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	get := func() (proxy.Client, error) {
		return nil, models.ErrModelStarting
	}
	start := time.Now()
	if _, err := waitForModelReady(ctx, get, "m", time.Second, time.Minute); err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if time.Since(start) > time.Second {
		t.Errorf("cancelled context should abort immediately, took %v", time.Since(start))
	}
}

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
	opts := executor.buildAgentOptions(req, logging.NewNopLogger(), nil)
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
	opts := executor.buildAgentOptions(req, logging.NewNopLogger(), nil)
	if opts.UseNativeTools != nil && *opts.UseNativeTools {
		t.Fatalf("expected UseNativeTools unset/false with no format in config, got %v", opts.UseNativeTools)
	}
}

// TestRecordRun_PersistsWarnings pins the run-ledger side of the tool-error
// classification: non-fatal tool failures (delivery channels down) are recorded
// as warnings on the run entry while Error stays empty — a warning-only run is
// still a success.
func TestRecordRun_PersistsWarnings(t *testing.T) {
	executor := &LLMTaskExecutor{}
	state := &models.AgentState{}
	outcome := runOutcome{
		req:      ExecuteRequest{WorkspaceID: "ws1", AutomationName: "nightly"},
		resp:     &ExecuteResponse{State: state},
		warnings: []string{"notify_user: telegram API error: status 401"},
	}

	executor.recordRun(outcome, "report body", "", 0)

	if len(state.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(state.History))
	}
	run := state.History[0]
	if run.Error != "" {
		t.Errorf("warning-only run must not be an error, got %q", run.Error)
	}
	if len(run.Warnings) != 1 || run.Warnings[0] != "notify_user: telegram API error: status 401" {
		t.Fatalf("warnings not persisted on the run entry: %+v", run.Warnings)
	}
	if state.LastRuns["nightly"] == nil || len(state.LastRuns["nightly"].Warnings) != 1 {
		t.Fatalf("warnings not persisted on LastRuns: %+v", state.LastRuns["nightly"])
	}
}

// memory_mode is opt-in per automation: unset/off leaves the agent without a
// memory store (the long-standing behaviour); hot gives it the store and turns
// on the frozen head-system hot-memory block.
func TestBuildAgentOptions_MemoryMode(t *testing.T) {
	store := newTestMemoryStore(t)
	cases := []struct {
		mode    models.MemoryMode
		wantHot bool
	}{
		{"", false},
		{models.MemoryModeOff, false},
		{models.MemoryModeHot, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode)+"_mode", func(t *testing.T) {
			executor := NewLLMTaskExecutor(&mockSvc{memoryStore: store}).(*LLMTaskExecutor)
			opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws", MemoryMode: tc.mode}, logging.NewNopLogger(), nil)
			if opts.EnableHotMemory != tc.wantHot {
				t.Errorf("EnableHotMemory = %v, want %v", opts.EnableHotMemory, tc.wantHot)
			}
			if gotStore := opts.MemoryStore != nil; gotStore != tc.wantHot {
				t.Errorf("MemoryStore set = %v, want %v", gotStore, tc.wantHot)
			}
		})
	}
}

func TestNewRunContext_StampsIdentityAndUnattended(t *testing.T) {
	ctx := newRunContext(context.Background(), "nightly", "run-42")
	if models.GetTaskName(ctx) != "nightly" || models.GetRunID(ctx) != "run-42" {
		t.Errorf("task/run id not stamped: %q / %q", models.GetTaskName(ctx), models.GetRunID(ctx))
	}
	if !models.IsUnattendedRun(ctx) {
		t.Error("an automation run is unattended: memory_update must default to session scope")
	}
}

// ── Execute end to end ──────────────────────────────────────────────────────
//
// buildAgentOptions is tested above, but the question that matters is whether the
// text a real automation run SENDS contains the memory. The chat path once enabled
// hot memory without ever handing the agent its store, and every unit test that
// injected the store directly stayed green. This drives the real Execute with a
// scripted model and records what it was sent.

type noopPublisher struct{}

func (noopPublisher) Publish(string, assistant.AgentEvent) {}
func (noopPublisher) Clear(string, assistant.EventChannel) {}

type probeTools struct{}

func (probeTools) ListTools(context.Context) ([]proxy.Tool, error) {
	return []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "probe_tool"}}}, nil
}
func (probeTools) GetSystemPrompt() (string, error) { return "", nil }
func (probeTools) UseNativeTools() bool             { return true }

type probeEngine struct{}

func (probeEngine) ExecuteTool(context.Context, proxy.ToolCall) (any, error) { return "ok", nil }

// recordingClient scripts a two-request run (one tool call, then an answer) and
// keeps the messages of every request it receives.
type recordingClient struct{ requests [][]proxy.Message }

func (c *recordingClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return nil, fmt.Errorf("streaming only")
}
func (c *recordingClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	c.requests = append(c.requests, append([]proxy.Message(nil), req.Messages...))
	ch := make(chan *proxy.ChatResponse, 1)
	if len(c.requests) == 1 {
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
			ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "probe_tool", Arguments: `{"path":"a"}`},
		}}}}}}
	} else {
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "# Done\nTask finished successfully"}}}}
	}
	close(ch)
	return ch, nil
}
func (c *recordingClient) ReasoningField() string { return proxy.DefaultReasoningField }

type execSvc struct {
	mockSvc
	client      *recordingClient
	persistence *persistence.WorkspaceManager
	guardrails  *guardrails.GuardrailEngine
	decisions   *assistant.GuardrailDecisionStore
}

func (s *execSvc) GetClientForModel(context.Context, string) (proxy.Client, error) {
	return s.client, nil
}
func (s *execSvc) ToolProvider() assistant.ToolProvider                      { return probeTools{} }
func (s *execSvc) Engine() assistant.Engine                                  { return probeEngine{} }
func (s *execSvc) GuardrailEngine() *guardrails.GuardrailEngine              { return s.guardrails }
func (s *execSvc) GuardrailDecisionStore() *assistant.GuardrailDecisionStore { return s.decisions }
func (s *execSvc) ProcessLogger(string) logging.Logger                       { return logging.NewNopLogger() }
func (s *execSvc) Logger() logging.Logger                                    { return logging.NewNopLogger() }
func (s *execSvc) Persistence() *persistence.WorkspaceManager                { return s.persistence }
func (s *execSvc) Events() assistant.EventPublisher                          { return noopPublisher{} }

func newExecSvc(t *testing.T, store *memory.Store) *execSvc {
	t.Helper()
	root := t.TempDir()
	resolver := storage.NewPathResolver(root, root, root)
	if err := os.MkdirAll(resolver.WorkspaceDir("ws"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &execSvc{
		mockSvc:     mockSvc{memoryStore: store, modelCfg: models.ModelConfig{Name: "m", MaxSteps: 5, MaxTokens: 512, ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}},
		client:      &recordingClient{},
		persistence: persistence.NewWorkspaceManager(resolver),
		guardrails:  guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, resolver, nil, nil),
		decisions:   assistant.NewGuardrailDecisionStore(),
	}
}

func TestExecute_HotMemoryReachesTheModelOnlyWhenTheAutomationOptsIn(t *testing.T) {
	cases := []struct {
		mode     models.MemoryMode
		wantFact bool
	}{
		{"", false},
		{models.MemoryModeOff, false},
		{models.MemoryModeHot, true},
	}
	for _, tc := range cases {
		t.Run("memory_mode="+string(tc.mode), func(t *testing.T) {
			store := newTestMemoryStore(t)
			ctx := context.Background()
			if _, err := store.Insert(ctx, "ws", memory.LongTerm, "codename", "The project codename is BLUEHERON-7.", []string{memory.HotTag}, "operator"); err != nil {
				t.Fatal(err)
			}
			svc := newExecSvc(t, store)

			req := ExecuteRequest{WorkspaceID: "ws", AutomationName: "nightly", TaskContent: "Say the codename.", Model: "m", MemoryMode: tc.mode}
			_, _ = NewLLMTaskExecutor(svc).Execute(ctx, req)

			if len(svc.client.requests) == 0 {
				t.Fatal("the model was never called")
			}
			for i, msgs := range svc.client.requests {
				head := msgs[0].Content
				if got := strings.Contains(head, "BLUEHERON-7"); got != tc.wantFact {
					t.Errorf("request %d: fact in the system message = %v, want %v", i+1, got, tc.wantFact)
				}
			}

			if err := store.FlushUsage(ctx); err != nil {
				t.Fatal(err)
			}
			all, _ := store.List(ctx, "ws", "", 10, 0)
			wantCount := 0
			if tc.wantFact {
				wantCount = 1 // once per run, however many turns it takes
			}
			if len(all) != 1 || all[0].InjectedCount != wantCount {
				t.Errorf("usage count = %+v, want %d", all, wantCount)
			}
		})
	}
}
