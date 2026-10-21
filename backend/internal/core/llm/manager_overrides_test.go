package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/models"
)

func overrideModels(t *testing.T) *llm.LLMRuntimeManager {
	t.Helper()
	m := llm.NewWithReapInterval(nil, "127.0.0.1", time.Minute, time.Hour)
	if err := m.AddModel(models.ModelConfig{
		Name: "local", Provider: models.ProviderLocal, Filename: "l.gguf", Path: "/models/l.gguf", Port: 9,
		Metadata: &models.ModelMetadata{Nctx: 16384, ContextLength: 16384},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.AddModel(models.ModelConfig{Name: "cloud", Provider: "openai", Filename: "gpt-x"}); err != nil {
		t.Fatal(err)
	}
	m.Sync()
	return m
}

// A per-model override from settings.yml is applied to the model it names. A local model keeps the output cap and
// context budget its window gives it (they are derived from the server, never overridden); a cloud model takes them.
func TestApplyModelOverrides_MergesTuning(t *testing.T) {
	yes, no := true, false
	override := models.ModelOverride{
		MaxSteps: 7, ContextBudget: 123456, MaxTokens: 4321, Temperature: 0.6, ToolCallFormat: "native",
		Prefill: &yes, ReasoningEnabled: &no, ReasoningBudget: 999, SlotTimeout: 33, TimeoutMinutes: 12, ICUWeight: 2.5,
		LoopStrategy: "react",
	}
	m := overrideModels(t)
	localBefore := findModel(t, m, "local")

	m.ApplyModelOverrides(map[string]models.ModelOverride{"local": override, "cloud": override, "ghost": override})

	local := findModel(t, m, "local")
	if local.MaxTokens != localBefore.MaxTokens || local.ContextBudget != localBefore.ContextBudget {
		t.Errorf("a local model's window-derived budgets were overridden: %d/%d, want %d/%d",
			local.MaxTokens, local.ContextBudget, localBefore.MaxTokens, localBefore.ContextBudget)
	}
	cloud := findModel(t, m, "cloud")
	if cloud.MaxTokens != 4321 || cloud.ContextBudget != 123456 {
		t.Errorf("cloud budgets: max_tokens %d context_budget %d", cloud.MaxTokens, cloud.ContextBudget)
	}
	for _, got := range []models.ModelConfig{local, cloud} {
		if got.MaxSteps != 7 || got.ToolCallFormat != "native" || got.Temperature != 0.6 || got.Prefill == nil || !*got.Prefill ||
			got.ReasoningEnabled == nil || *got.ReasoningEnabled || got.ReasoningBudget != 999 || got.SlotTimeout != 33 ||
			got.TimeoutMinutes != 12 || got.LoopStrategy != "react" {
			t.Errorf("%s: behaviour overrides not applied: %+v", got.Name, got)
		}
	}
	if cloud.ProviderConfig == nil || cloud.ProviderConfig.InternalCreditWeight != 2.5 {
		t.Errorf("credit weight not applied: %+v", cloud.ProviderConfig)
	}

	// An override field that is unset leaves the model's own value alone.
	m.ApplyModelOverrides(map[string]models.ModelOverride{"cloud": {}})
	if again := findModel(t, m, "cloud"); again.MaxSteps != 7 || again.Temperature != 0.6 {
		t.Errorf("an empty override changed the model: %+v", again)
	}
}

// Sync re-derives every model's budgets from its stored metadata, and starts the next server start's serving-context
// check afresh: the window is probed again and the grace clock begins again.
func TestSync_RederivesBudgetsAndRestartsTheServingContextCheck(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" {
			probes.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":0,"n_ctx":16384}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	port := portOf(t, server.URL)

	m := llm.NewWithReapInterval(nil, "127.0.0.1", time.Minute, time.Hour)
	if err := m.AddModel(models.ModelConfig{
		Name: "qwen", Provider: models.ProviderLocal, Filename: "q.gguf", Path: "/models/q.gguf", Port: port,
		MaxTokens: 87381, ContextBudget: 699052, // stale values that Sync must not keep
		Metadata: &models.ModelMetadata{Nctx: 16384, ContextLength: 16384},
	}); err != nil {
		t.Fatal(err)
	}

	m.Sync()
	cfg := findModel(t, m, "qwen")
	if cfg.MaxTokens != 16384/3 || cfg.ContextBudget != 43692 {
		t.Fatalf("budgets after Sync: max_tokens %d context_budget %d; want %d and 43692", cfg.MaxTokens, cfg.ContextBudget, 16384/3)
	}

	if !m.ReconcileLocalServingContext(context.Background(), "qwen") {
		t.Fatal("not settled")
	}
	first := probes.Load()
	m.ReconcileLocalServingContext(context.Background(), "qwen")
	if probes.Load() != first {
		t.Fatal("probed again without a Sync or a stop")
	}
	m.Sync()
	m.ReconcileLocalServingContext(context.Background(), "qwen")
	if probes.Load() == first {
		t.Error("Sync did not start the serving-context check afresh")
	}
}

func TestSync_RestartsTheGraceClock(t *testing.T) {
	old := llm.ServingContextGrace
	llm.ServingContextGrace = 40 * time.Millisecond
	defer func() { llm.ServingContextGrace = old }()

	ls := newLoadingServer(t, 16384) // never finishes loading
	m := llm.NewWithReapInterval(nil, "127.0.0.1", time.Minute, time.Hour)
	if err := m.AddModel(staleQwen(ls.port(t))); err != nil {
		t.Fatal(err)
	}
	m.ReconcileLocalServingContext(context.Background(), "qwen") // starts the clock
	time.Sleep(60 * time.Millisecond)
	m.Sync()
	if m.ReconcileLocalServingContext(context.Background(), "qwen") {
		t.Error("the grace clock was not restarted by Sync")
	}
}

func portOf(t *testing.T, url string) int {
	t.Helper()
	port, err := strconv.Atoi(strings.TrimPrefix(url, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}
	return port
}
