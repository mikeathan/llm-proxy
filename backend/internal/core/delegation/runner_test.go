package delegation

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/usage"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// scriptedClient streams a search call on its first request and an answer after, recording what it was offered.
type scriptedClient struct {
	mu       sync.Mutex
	requests []proxy.ChatRequest
}

func (c *scriptedClient) Chat(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return &proxy.ChatResponse{}, nil
}
func (c *scriptedClient) ReasoningField() string { return proxy.ReasoningFieldBudget }
func (c *scriptedClient) Stream(_ context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	n := len(c.requests)
	c.mu.Unlock()
	ch := make(chan *proxy.ChatResponse, 1)
	if n == 1 {
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
			ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: models.ToolInternetSearch, Arguments: `{"query":"rust 2026"}`},
		}}}}}}
	} else {
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Rust 2026 shipped in March. https://example.com/rust"}}}}
	}
	close(ch)
	return ch, nil
}

// allTools offers every kind of tool, so the test can see the child keeps only the web ones.
type allTools struct{}

func (allTools) UseNativeTools() bool { return true }
func (allTools) ListTools(context.Context) ([]proxy.Tool, error) {
	var out []proxy.Tool
	for _, name := range []string{models.ToolInternetSearch, models.ToolNetworkFetch, models.ToolFileRead, models.ToolTerminalExecute, models.ToolNetworkScan, models.ToolMemoryUpdate} {
		out = append(out, proxy.Tool{Type: "function", Function: proxy.FunctionSchema{Name: name}})
	}
	return out, nil
}

// recordingEngine answers every call and notes whether it ran unattended.
type recordingEngine struct {
	mu         sync.Mutex
	calls      []string
	unattended bool
}

func (e *recordingEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, call.Function.Name)
	e.unattended = models.IsUnattendedRun(ctx)
	return "result for " + call.Function.Name, nil
}

type stubDeps struct {
	settings *models.AgentConfig
	lanes    map[string]runlane.LaneKey
	client   *scriptedClient
	engine   *recordingEngine
	pm       *persistence.WorkspaceManager
	catalog  *profile.Catalog
}

func newStubDeps(t *testing.T) *stubDeps {
	t.Helper()
	tmp := t.TempDir()
	return &stubDeps{
		settings: &models.AgentConfig{SubagentModel: "cloud-m"},
		lanes:    map[string]runlane.LaneKey{"cloud-m": runlane.LaneCloud, "local-m": runlane.LaneLocal},
		client:   &scriptedClient{},
		engine:   &recordingEngine{},
		pm:       persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp)),
		catalog:  profile.MustCatalog("", logging.NewNopLogger()),
	}
}

func (d *stubDeps) ModelConfig(name string) (models.ModelConfig, bool) {
	return models.ModelConfig{Name: name}, true
}

// GuardrailEngine allows web search and page fetch, as a configured app does.
func (d *stubDeps) GuardrailEngine() *guardrails.GuardrailEngine {
	return guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Search: models.SearchGuardrailsConfig{Enabled: true}, Network: models.NetworkGuardrailsConfig{Enabled: true}}
	}, storage.NewPathResolver("", "", ""), nil, nil)
}
func (d *stubDeps) GuardrailDecisionStore() *assistant.GuardrailDecisionStore {
	return assistant.NewGuardrailDecisionStore()
}
func (d *stubDeps) Orchestrator() *orchestrator.Orchestrator { return nil }
func (d *stubDeps) MemoryStore() *memory.Store               { return nil }
func (d *stubDeps) MemorySettings() *models.MemoryConfig     { return nil }
func (d *stubDeps) AgentSettings() *models.AgentConfig       { return d.settings }
func (d *stubDeps) SubRunner() assistant.SubRunner           { return nil }
func (d *stubDeps) Profiles() *profile.Catalog               { return d.catalog }
func (d *stubDeps) GetClientForModel(context.Context, string) (proxy.Client, error) {
	return d.client, nil
}
func (d *stubDeps) EffectiveToolCallFormat(context.Context, string) string { return "native" }
func (d *stubDeps) ToolProvider() assistant.ToolProvider                   { return allTools{} }
func (d *stubDeps) Engine() assistant.Engine                               { return d.engine }
func (d *stubDeps) LaneKeyFor(model string) runlane.LaneKey                { return d.lanes[model] }
func (d *stubDeps) ProcessLogger(string) logging.Logger                    { return logging.NewNopLogger() }

func task(agent string, events *[]assistant.SubagentEvent) assistant.SubRunRequest {
	var mu sync.Mutex
	return assistant.SubRunRequest{WorkspaceID: "ws-1", Agent: agent, Task: "What shipped in Rust 2026?", Context: "this year only",
		Emit: func(ev assistant.SubagentEvent) { mu.Lock(); *events = append(*events, ev); mu.Unlock() }}
}

// A helper runs on the cloud model with web tools only, unattended, in its own run and tracker, and reports coarse
// progress to the parent.
func TestRunner_RunsAHelperWithWebToolsOnly(t *testing.T) {
	deps := newStubDeps(t)
	r := New(deps)
	parentCtx := usage.WithTracker(models.WithRunID(context.Background(), "parent-run"))
	var events []assistant.SubagentEvent
	res := r.Run(parentCtx, task("web-researcher", &events))

	if res.Status != assistant.SubRunDone || !strings.Contains(res.Summary, "shipped in March") || res.Model != "cloud-m" {
		t.Fatalf("result = %+v", res)
	}
	if res.RunID == "" || res.RunID == "parent-run" {
		t.Errorf("the child runs as its own run, got id %q", res.RunID)
	}
	var offered []string
	for _, tool := range deps.client.requests[0].Tools {
		offered = append(offered, tool.Function.Name)
	}
	slices.Sort(offered)
	if strings.Join(offered, ",") != models.ToolNetworkFetch+","+models.ToolInternetSearch {
		t.Errorf("the child was offered %v, want only the web tools", offered)
	}
	if !deps.engine.unattended || !slices.Equal(deps.engine.calls, []string{models.ToolInternetSearch}) {
		t.Errorf("engine calls %v (unattended %v)", deps.engine.calls, deps.engine.unattended)
	}
	first := deps.client.requests[0].Messages
	if user := first[len(first)-1].Content; !strings.Contains(user, "What shipped in Rust 2026?") || !strings.Contains(user, "this year only") {
		t.Errorf("the helper's task message = %q", user)
	}
	var kinds []string
	for _, ev := range events {
		kinds = append(kinds, string(ev.Kind))
	}
	if strings.Join(kinds, ",") != "started,tool_call,done" {
		t.Errorf("progress = %v", kinds)
	}
	if p, c := usage.FromContext(parentCtx).ReportedUsage(); p != 0 || c != 0 {
		t.Errorf("the child's tokens reached the parent's tracker: %d, %d", p, c)
	}
}

// Only what the parent wrote into the call leaves the machine: the workspace's own notes (AGENTS.md) are not in a
// cloud helper's prompt.
func TestRunner_SendsNoWorkspaceNotes(t *testing.T) {
	deps := newStubDeps(t)
	if err := deps.pm.WriteTaskFile("ws-1", models.RulesFilename, "PRIVATE-NOTE-7781: the deploy host is vertex"); err != nil {
		t.Fatal(err)
	}
	var events []assistant.SubagentEvent
	if res := New(deps).Run(context.Background(), task("web-researcher", &events)); res.Status != assistant.SubRunDone {
		t.Fatalf("result = %+v", res)
	}
	for _, m := range deps.client.requests[0].Messages {
		if strings.Contains(m.Content, "PRIVATE-NOTE-7781") {
			t.Fatalf("the workspace notes reached the cloud helper in its %s message", m.Role)
		}
	}
}

// Without a cloud model, or for an unknown helper, nothing runs.
func TestRunner_Refuses(t *testing.T) {
	cases := []struct {
		name, model, agent, want string
	}{
		{"no model set", "", "web-researcher", "no sub-agent model"},
		{"a local model", "local-m", "web-researcher", "not a cloud model"},
		{"a model the classifier cannot place", "mystery-m", "web-researcher", "not a cloud model"},
		{"an unknown helper", "cloud-m", "nobody", "no helper profile"},
	}
	for _, tc := range cases {
		deps := newStubDeps(t)
		deps.settings.SubagentModel = tc.model
		r := New(deps)
		var events []assistant.SubagentEvent
		res := r.Run(context.Background(), task(tc.agent, &events))
		if res.Status != assistant.SubRunRefused || !strings.Contains(res.Error, tc.want) || len(deps.client.requests) != 0 {
			t.Errorf("%s: %+v (requests %d)", tc.name, res, len(deps.client.requests))
		}
		if tc.agent != "nobody" && r.Enabled() {
			t.Errorf("%s: Enabled() = true", tc.name)
		}
	}
}

// A full pool reports busy after its wait; a cancelled parent reports stopped.
func TestRunner_BusyAndStopped(t *testing.T) {
	deps := newStubDeps(t)
	deps.settings.SubagentConcurrency = 1
	r := New(deps)
	r.slotWait = 20 * time.Millisecond
	release, err := r.pool.acquire(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var events []assistant.SubagentEvent
	if res := r.Run(context.Background(), task("web-researcher", &events)); res.Status != assistant.SubRunBusy {
		t.Errorf("a full pool: %+v", res)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := r.Run(ctx, task("web-researcher", &events)); res.Status != assistant.SubRunStopped {
		t.Errorf("a cancelled parent: %+v", res)
	}
}
