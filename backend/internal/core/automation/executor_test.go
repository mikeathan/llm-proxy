package automation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
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
	result := executor.buildPrompt(taskContent, req, taskRunner)

	if !strings.Contains(result, taskContent) {
		t.Errorf("expected buildPrompt output to contain original task content, got:\n%s", result)
	}
	if !strings.Contains(result, prompts.AutomationMarker) {
		t.Errorf("expected buildPrompt output to contain AutomationMarker, got:\n%s", result)
	}
}

// An automation whose memory is active is told to consult memory_search before it
// acts; with memory off, no store to search, or an allow-list that excludes the
// tool, the instruction is withheld — the model must never be told to call a tool
// it was not given (and the allow-list is never widened for it).
func TestBuildPrompt_MemorySearchInstruction(t *testing.T) {
	store := newTestMemoryStore(t)
	for _, tc := range []struct {
		name    string
		svc     *mockSvc
		mode    models.MemoryMode
		allowed []string
		want    bool
	}{
		{"memory on with a store", &mockSvc{memoryStore: store}, models.MemoryModeOn, nil, true},
		{"memory off", &mockSvc{memoryStore: store}, models.MemoryModeOff, nil, false},
		{"on but no store", &mockSvc{}, models.MemoryModeOn, nil, false},
		{"allow-list without memory_search", &mockSvc{memoryStore: store}, models.MemoryModeOn, []string{"read_file"}, false},
		{"allow-list with memory_search", &mockSvc{memoryStore: store}, models.MemoryModeOn, []string{"read_file", models.ToolMemorySearch}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewLLMTaskExecutor(tc.svc).(*LLMTaskExecutor)
			got := executor.buildPrompt("do the thing", ExecuteRequest{WorkspaceID: "ws", TaskFile: "t.md", MemoryMode: tc.mode, AllowedTools: tc.allowed}, taskRunner)
			if has := strings.Contains(got, models.ToolMemorySearch); has != tc.want {
				t.Errorf("prompt mentions %s = %v, want %v:\n%s", models.ToolMemorySearch, has, tc.want, got)
			}
		})
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
// taskRunner is the default profile for automations, as a run without a chosen profile resolves it.
var taskRunner = profile.MustCatalog("", logging.NewNopLogger()).Resolve(profile.KindAutomation)

type mockSvc struct {
	agent       *models.AgentConfig // nil: the adaptive result cap defaults to on
	memoryStore *memory.Store
	memory      *models.MemoryConfig
	modelCfg    models.ModelConfig
	profiles    *profile.Catalog // the built-ins unless a test sets its own
}

func (m *mockSvc) Profiles() *profile.Catalog {
	if m.profiles == nil {
		m.profiles = profile.MustCatalog("", logging.NewNopLogger())
	}
	return m.profiles
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
func (m *mockSvc) MemorySettings() *models.MemoryConfig                                 { return m.memory }
func (m *mockSvc) AgentSettings() *models.AgentConfig                                   { return m.agent }
func (m *mockSvc) SubRunner() assistant.SubRunner                                       { return nil }
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
	opts := executor.buildAgentOptions(req, taskRunner, logging.NewNopLogger(), nil)
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
	opts := executor.buildAgentOptions(req, taskRunner, logging.NewNopLogger(), nil)
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

// memory_mode overrides the global automation default: inherit follows it, on/off beat it. Hot memory needs the
// store and turns on the frozen head-system block; without it the agent has no memory store at all.
func TestBuildAgentOptions_MemoryMode(t *testing.T) {
	store := newTestMemoryStore(t)
	cases := []struct {
		mode          models.MemoryMode
		globalDefault bool
		wantHot       bool
	}{
		{models.MemoryModeInherit, false, false},
		{models.MemoryModeInherit, true, true},
		{models.MemoryModeOff, true, false},
		{models.MemoryModeOn, false, true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("mode=%q/global=%v", tc.mode, tc.globalDefault), func(t *testing.T) {
			svc := &mockSvc{memoryStore: store, memory: &models.MemoryConfig{AutomationHot: new(tc.globalDefault)}}
			executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
			opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws", MemoryMode: tc.mode}, taskRunner, logging.NewNopLogger(), nil)
			if opts.EnableHotMemory != tc.wantHot {
				t.Errorf("EnableHotMemory = %v, want %v", opts.EnableHotMemory, tc.wantHot)
			}
			if gotStore := opts.MemoryStore != nil; gotStore != tc.wantHot {
				t.Errorf("MemoryStore set = %v, want %v", gotStore, tc.wantHot)
			}
		})
	}
}

// A profile's memory setting is a default beneath the automation's own and above the global one.
func TestBuildAgentOptions_ProfileMemoryDefault(t *testing.T) {
	store := newTestMemoryStore(t)
	cases := []struct {
		name    string
		profile models.MemoryMode
		own     models.MemoryMode
		global  bool
		wantHot bool
	}{
		{"profile on beats a global off", models.MemoryModeOn, models.MemoryModeInherit, false, true},
		{"profile off beats a global on", models.MemoryModeOff, models.MemoryModeInherit, true, false},
		{"the automation's off beats the profile's on", models.MemoryModeOn, models.MemoryModeOff, true, false},
		{"the automation's on beats the profile's off", models.MemoryModeOff, models.MemoryModeOn, false, true},
		{"no opinion anywhere follows the global default", models.MemoryModeInherit, models.MemoryModeInherit, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockSvc{memoryStore: store, memory: &models.MemoryConfig{AutomationHot: new(tc.global)}}
			executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
			opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws", MemoryMode: tc.own}, profile.Profile{ID: "p", Memory: tc.profile}, logging.NewNopLogger(), nil)
			if opts.EnableHotMemory != tc.wantHot {
				t.Errorf("EnableHotMemory = %v, want %v", opts.EnableHotMemory, tc.wantHot)
			}
		})
	}
}

// The run's agent carries the resolved profile, and the prompt does not promise a memory tool the profile hides.
func TestBuildAgentOptions_CarriesTheProfileAndItsToolSet(t *testing.T) {
	store := newTestMemoryStore(t)
	executor := NewLLMTaskExecutor(&mockSvc{memoryStore: store}).(*LLMTaskExecutor)
	filesOnly := profile.Profile{ID: "files-only", Tools: []string{models.CategoryFileSystem}}

	opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws"}, filesOnly, logging.NewNopLogger(), nil)
	if opts.Profile.ID != "files-only" {
		t.Errorf("agent profile = %q, want files-only", opts.Profile.ID)
	}
	prompt := executor.buildPrompt("do it", ExecuteRequest{WorkspaceID: "ws", TaskFile: "t.md", MemoryMode: models.MemoryModeOn}, filesOnly)
	if strings.Contains(prompt, models.ToolMemorySearch) {
		t.Errorf("the profile hides the memory tools, so the prompt must not tell the run to use them:\n%s", prompt)
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
func (probeTools) UseNativeTools() bool { return true }

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
		{models.MemoryModeOn, true},
	}
	for _, tc := range cases {
		t.Run("memory_mode="+string(tc.mode), func(t *testing.T) {
			store := newTestMemoryStore(t)
			ctx := context.Background()
			if _, err := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws", MemoryType: memory.LongTerm, Title: "codename", Content: "The project codename is BLUEHERON-7.", Tags: []string{memory.HotTag}, Source: "operator"}); err != nil {
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

// The text an automation run sends carries its profile's behaviour: Task runner unless it names another, and the
// heartbeat's Watcher. The old tool-first rules are no longer in every run.
func TestExecute_SendsTheRunsProfileBehaviour(t *testing.T) {
	cases := []struct {
		name, profile, wantMarker string
	}{
		{"default for automations", "", "You run unattended"},
		{"a named profile", "research", "find out what is true"},
		{"the heartbeat's profile", models.HeartbeatProfile, "frequent check unattended"},
		{"an unknown profile falls back to the default", "deleted-profile", "You run unattended"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newExecSvc(t, nil)
			req := ExecuteRequest{WorkspaceID: "ws", AutomationName: "nightly", TaskContent: "Do it.", Model: "m", Profile: tc.profile}
			_, _ = NewLLMTaskExecutor(svc).Execute(context.Background(), req)

			if len(svc.client.requests) == 0 {
				t.Fatal("the model was never called")
			}
			head := svc.client.requests[0][0].Content
			if !strings.Contains(head, tc.wantMarker) {
				t.Errorf("system message lacks %q:\n%s", tc.wantMarker, head)
			}
			if !strings.Contains(head, "INSTRUCTION BOUNDARY") {
				t.Error("the code-owned boundary rules are in every run")
			}
			for _, gone := range []string{"BEST PRACTICE", "verifying your environment"} {
				if strings.Contains(head, gone) {
					t.Errorf("the old tool-first line %q must be gone", gone)
				}
			}
		})
	}
}

func TestApplyPulseLogic(t *testing.T) {
	const header = "⏱ **Duration:** 1s\n\n### Final Report\n\n"
	cases := []struct {
		name, report string
		wantQuiet    bool
	}{
		{"bare marker", "HEARTBEAT_OK", true},
		{"bold marker", "**HEARTBEAT_OK**", true},
		{"an alert that merely mentions the marker stays visible", "Not HEARTBEAT_OK: GPT-6 shipped", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &ExecuteResponse{Output: header + tc.report, Report: tc.report, State: &models.AgentState{}}
			ApplyPulseLogic(resp)
			if quiet := resp.Output == ""; quiet != tc.wantQuiet {
				t.Errorf("output blanked = %v, want %v", quiet, tc.wantQuiet)
			}
			if got := !resp.State.LastPulse.IsZero(); got != tc.wantQuiet {
				t.Errorf("LastPulse set = %v, want %v", got, tc.wantQuiet)
			}
		})
	}
}

// One run, one id: the history entry carries the id the run context was given, and the response reports it so the
// dispatcher can find that run again after delivery.
func TestRecordRun_UsesTheRunIDAndReportsIt(t *testing.T) {
	state := &models.AgentState{}
	resp := &ExecuteResponse{State: state}
	e := NewLLMTaskExecutor(&mockSvc{}).(*LLMTaskExecutor)
	e.recordRun(runOutcome{req: ExecuteRequest{AutomationName: "nightly"}, resp: resp, runID: "run_ctx_1"}, "out", "", time.Second)
	if len(state.History) != 1 || state.History[0].ID != "run_ctx_1" || resp.RunID != "run_ctx_1" {
		t.Fatalf("history id = %q, resp.RunID = %q, want run_ctx_1 for both", state.History[0].ID, resp.RunID)
	}
}

// recordingPublisher keeps what is published, to check events are tagged.
type recordingPublisher struct {
	mu     sync.Mutex
	events []assistant.AgentEvent
}

func (r *recordingPublisher) Publish(_ string, ev assistant.AgentEvent) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}
func (r *recordingPublisher) Clear(string, assistant.EventChannel) {}

type publishingSvc struct {
	mockSvc
	pub       *recordingPublisher
	decisions *assistant.GuardrailDecisionStore
}

func (s *publishingSvc) GuardrailDecisionStore() *assistant.GuardrailDecisionStore {
	return s.decisions
}

func (s *publishingSvc) Events() assistant.EventPublisher { return s.pub }

// The agent's observer, the guardrail prompt and the run's concluding message all
// publish for one run; each must name the automation it belongs to.
func TestRunEvents_NameTheirAutomation(t *testing.T) {
	svc := &publishingSvc{pub: &recordingPublisher{}}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	req := ExecuteRequest{WorkspaceID: "ws", AutomationName: "daily-llm-news"}

	opts := executor.buildAgentOptions(req, taskRunner, logging.NewNopLogger(), nil)
	opts.Observer(assistant.AgentEvent{Type: assistant.EventToolCall, Channel: assistant.ChannelAutomation})
	executor.publishRunEvent(req, assistant.AgentEvent{Type: assistant.EventMessage, Channel: assistant.ChannelAutomation})

	if len(svc.pub.events) != 2 {
		t.Fatalf("published %d events, want 2", len(svc.pub.events))
	}
	for _, ev := range svc.pub.events {
		if ev.Automation != "daily-llm-news" {
			t.Errorf("%s event has Automation %q", ev.Type, ev.Automation)
		}
	}
}

// An approval prompt in an automation run is published as a blocked/invalidated
// pair; both must name the automation, or the console of that automation never
// sees the invalidation and its banner stays up.
func TestGuardrailPrompt_PairNamesTheAutomation(t *testing.T) {
	svc := &publishingSvc{pub: &recordingPublisher{}, decisions: assistant.NewGuardrailDecisionStore()}
	executor := NewLLMTaskExecutor(svc).(*LLMTaskExecutor)
	opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws", AutomationName: "daily-llm-news"}, taskRunner, logging.NewNopLogger(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = opts.GuardrailDecisionHandler(ctx, assistant.GuardrailBlockedPayload{DecisionID: "d1", Tool: "run_terminal", Category: "terminal"})

	types := map[assistant.AgentEventType]bool{}
	for _, ev := range svc.pub.events {
		types[ev.Type] = true
		if ev.Automation != "daily-llm-news" {
			t.Errorf("%s event has Automation %q", ev.Type, ev.Automation)
		}
	}
	if !types[assistant.EventGuardrailBlocked] || !types[assistant.EventGuardrailInvalidated] {
		t.Errorf("published %v, want a blocked and an invalidated event", types)
	}
}

// An unattended run reads the same files and pages as a chat, so it follows the same adaptive-result-cap setting.
func TestBuildAgentOptions_AdaptiveResultCapFollowsTheSetting(t *testing.T) {
	for name, agent := range map[string]*models.AgentConfig{
		"unset is on":       nil,
		"explicit on":       {AdaptiveResultCap: new(true)},
		"explicit off":      {AdaptiveResultCap: new(false)},
		"empty block is on": {},
	} {
		want := agent == nil || agent.AdaptiveResultCap == nil || *agent.AdaptiveResultCap
		t.Run(name, func(t *testing.T) {
			executor := NewLLMTaskExecutor(&mockSvc{agent: agent}).(*LLMTaskExecutor)
			opts := executor.buildAgentOptions(ExecuteRequest{WorkspaceID: "ws"}, taskRunner, logging.NewNopLogger(), nil)
			if opts.AdaptiveResultCap != want {
				t.Errorf("AdaptiveResultCap = %v, want %v", opts.AdaptiveResultCap, want)
			}
		})
	}
}

// autoOptionsView mirrors assistant's optionsView (test helpers cannot cross packages): the part of AgentOptions a
// run's settings decide, with pointers and functions reduced to whether they are set.
type autoOptionsView struct {
	MaxSteps, ContextBudget, MaxResponseTokens, ReasoningBudget, MaxPlanSteps int
	ReasoningEnabled, UseNativeTools                                          string
	Temperature                                                               float64
	WorkspaceID, ModelName, ProviderType, ConversationID, ProfileID           string
	GuardrailTimeoutBehavior                                                  string
	WorkloadClass                                                             models.WorkloadClass
	LoopStrategy                                                              assistant.LoopStrategyName
	Channel                                                                   assistant.EventChannel
	RunNetworkScope                                                           models.NetworkScope
	UsePrefill, EnableHotMemory, AdaptiveResultCap                            bool
	HasMemoryStore, HasGuardrails, HasObserver, HasDecisionHandler            bool
	HasLogger, HasOrchestrator                                                bool
	AllowedTools, ExcludedTools                                               []string
	GlobalTimeout, ToolTimeout, FilesystemToolTimeout, MaxPlanDuration        time.Duration
	GuardrailTimeout, GuardrailApprovalTimeout                                time.Duration
}

func autoBoolView(b *bool) string {
	if b == nil {
		return "unset"
	}
	return fmt.Sprint(*b)
}

func viewAutoOptions(o assistant.AgentOptions) autoOptionsView {
	return autoOptionsView{
		MaxSteps: o.MaxSteps, ContextBudget: o.ContextBudget, MaxResponseTokens: o.MaxResponseTokens,
		ReasoningBudget: o.ReasoningBudget, MaxPlanSteps: o.MaxPlanSteps,
		ReasoningEnabled: autoBoolView(o.ReasoningEnabled), UseNativeTools: autoBoolView(o.UseNativeTools),
		Temperature: o.Temperature, WorkspaceID: o.WorkspaceID, ModelName: o.ModelName, ProviderType: o.ProviderType,
		ConversationID: o.ConversationID, ProfileID: o.Profile.ID, GuardrailTimeoutBehavior: o.GuardrailTimeoutBehavior,
		WorkloadClass: o.WorkloadClass, LoopStrategy: o.LoopStrategy, Channel: o.Channel, RunNetworkScope: o.RunNetworkScope,
		UsePrefill: o.UsePrefill, EnableHotMemory: o.EnableHotMemory, AdaptiveResultCap: o.AdaptiveResultCap,
		HasMemoryStore: o.MemoryStore != nil, HasGuardrails: o.Guardrails != nil, HasObserver: o.Observer != nil,
		HasDecisionHandler: o.GuardrailDecisionHandler != nil, HasLogger: o.Logger != nil, HasOrchestrator: o.Orchestrator != nil,
		AllowedTools: o.AllowedTools, ExcludedTools: o.ExcludedTools,
		GlobalTimeout: o.GlobalTimeout, ToolTimeout: o.ToolTimeout, FilesystemToolTimeout: o.FilesystemToolTimeout,
		MaxPlanDuration: o.MaxPlanDuration, GuardrailTimeout: o.GuardrailTimeout, GuardrailApprovalTimeout: o.GuardrailApprovalTimeout,
	}
}

// An automation sets the default step budget itself, attaches the memory store only when its memory is on, carries an
// allow list and no conversation id. (The network scope is resolved later, in Execute.)
func TestAutomationAgentOptions_Characterisation(t *testing.T) {
	store := newTestMemoryStore(t)
	full := models.ModelConfig{
		Name: "cloud-m", Provider: "openai", WorkloadClass: models.WorkloadCloud, LoopStrategy: models.LoopStrategyPlanExecute,
		MaxSteps: 21, ContextBudget: 9000, MaxTokens: 1500, ReasoningBudget: 700, ReasoningEnabled: new(true),
		Temperature: 0.3, ToolCallFormat: "native", Prefill: new(true), TimeoutMinutes: 7, ToolTimeoutSeconds: 40,
		FilesystemToolTimeoutSeconds: 9, MaxPlanDurationMinutes: 11, MaxPlanSteps: 13, GuardrailTimeoutSeconds: 3,
		GuardrailTimeoutBehavior: "fail-closed", GuardrailApprovalTimeoutSecs: 90,
	}
	research := profile.MustCatalog("", logging.NewNopLogger()).Resolve(profile.KindAutomation, "research")
	base := autoOptionsView{
		MaxSteps: assistant.DefaultMaxSteps, ReasoningEnabled: "unset", UseNativeTools: "unset", WorkspaceID: "ws",
		ProfileID: "task-runner", Channel: assistant.ChannelAutomation, AdaptiveResultCap: true,
		HasObserver: true, HasDecisionHandler: true, HasLogger: true,
	}
	cases := []struct {
		name    string
		svc     *mockSvc
		req     ExecuteRequest
		profile profile.Profile
		want    func(autoOptionsView) autoOptionsView
	}{
		{name: "defaults, no model", svc: &mockSvc{memoryStore: store}, req: ExecuteRequest{WorkspaceID: "ws"}, profile: taskRunner,
			want: func(v autoOptionsView) autoOptionsView { return v }},
		{name: "full model config, loop override", svc: &mockSvc{memoryStore: store, modelCfg: full},
			req: ExecuteRequest{WorkspaceID: "ws", Model: "cloud-m", LoopStrategy: string(models.LoopStrategyReact)}, profile: taskRunner,
			want: func(v autoOptionsView) autoOptionsView {
				v.MaxSteps, v.ContextBudget, v.MaxResponseTokens, v.ReasoningBudget, v.MaxPlanSteps = 21, 9000, 1500, 700, 13
				v.ReasoningEnabled, v.UseNativeTools, v.Temperature, v.UsePrefill = "true", "true", 0.3, true
				v.ModelName, v.ProviderType, v.WorkloadClass, v.GuardrailTimeoutBehavior = "cloud-m", "openai", models.WorkloadCloud, "fail-closed"
				v.LoopStrategy = assistant.LoopReact // the automation's own choice beats the model's plan_execute
				v.GlobalTimeout, v.ToolTimeout, v.FilesystemToolTimeout = 7*time.Minute, 40*time.Second, 9*time.Second
				v.MaxPlanDuration, v.GuardrailTimeout, v.GuardrailApprovalTimeout = 11*time.Minute, 3*time.Second, 90*time.Second
				return v
			}},
		{name: "allowed tools, memory on, research profile", svc: &mockSvc{memoryStore: store, agent: &models.AgentConfig{AdaptiveResultCap: new(false)}},
			req: ExecuteRequest{WorkspaceID: "ws", AllowedTools: []string{models.ToolFileRead}, MemoryMode: models.MemoryModeOn}, profile: research,
			want: func(v autoOptionsView) autoOptionsView {
				v.ProfileID, v.AllowedTools, v.AdaptiveResultCap = "research", []string{models.ToolFileRead}, false
				v.EnableHotMemory, v.HasMemoryStore = true, true
				return v
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewLLMTaskExecutor(tc.svc).(*LLMTaskExecutor)
			got := viewAutoOptions(executor.buildAgentOptions(tc.req, tc.profile, logging.NewNopLogger(), nil))
			if want := tc.want(base); !reflect.DeepEqual(got, want) {
				t.Errorf("resolved options changed\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}
