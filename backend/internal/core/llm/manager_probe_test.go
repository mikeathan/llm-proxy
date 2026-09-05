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

// newProbeManager builds a runtime manager whose provider infrastructure
// client is a plain local client (production injects the shared pooled doer at
// bootstrap; a fresh registrar has none).
func newProbeManager(t *testing.T) *llm.LLMRuntimeManager {
	t.Helper()
	m := llm.NewWithReapInterval(nil, "127.0.0.1", time.Minute, time.Hour)
	m.Registrar().SetHTTPDoer(&http.Client{Timeout: 10 * time.Second})
	return m
}

// findModel returns the stored runtime config for name.
func findModel(t *testing.T, m *llm.LLMRuntimeManager, name string) models.ModelConfig {
	t.Helper()
	for _, cfg := range m.ListModels() {
		if cfg.Name == name {
			return cfg
		}
	}
	t.Fatalf("model %q not found", name)
	return models.ModelConfig{}
}

// TestEffectiveToolCallFormat_LocalNativeProbe verifies a local OpenAI-
// compatible model with an unset tool_call_format is probed once, resolved to
// "native" when the endpoint returns native tool calls, cached on the stored
// config, and visible through ListModels (the agent-config path).
func TestEffectiveToolCallFormat_LocalNativeProbe(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
	}))
	defer server.Close()

	m := newProbeManager(t)
	if err := m.AddModel(models.ModelConfig{
		Name:     "local-model",
		Provider: "openai",
		Filename: "some-model-id",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	if got := m.EffectiveToolCallFormat(context.Background(), "local-model"); got != "native" {
		t.Fatalf("expected native from probe, got %q", got)
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("expected exactly 1 probe, got %d", n)
	}
	// Second resolution must be served from the persisted config.
	if got := m.EffectiveToolCallFormat(context.Background(), "local-model"); got != "native" {
		t.Fatalf("expected cached native, got %q", got)
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("expected no re-probe, got %d probes", n)
	}
	// The resolved format must reach the agent-config path.
	if cfg := findModel(t, m, "local-model"); cfg.ToolCallFormat != "native" {
		t.Fatalf("expected stored ToolCallFormat native, got %q", cfg.ToolCallFormat)
	}
}

// TestEffectiveToolCallFormat_NoNativeSupport verifies a server that only
// returns plain text keeps the model in XML text mode (empty format).
func TestEffectiveToolCallFormat_NoNativeSupport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"I cannot use tools."}}]}`))
	}))
	defer server.Close()

	m := newProbeManager(t)
	_ = m.AddModel(models.ModelConfig{
		Name:     "plain-model",
		Provider: "openai",
		Filename: "some-model-id",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	})

	if got := m.EffectiveToolCallFormat(context.Background(), "plain-model"); got != "" {
		t.Fatalf("expected empty (XML mode), got %q", got)
	}
}

// TestEffectiveToolCallFormat_ExplicitOverride verifies explicit
// tool_call_format config always wins and no probe is issued.
func TestEffectiveToolCallFormat_ExplicitOverride(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	m := newProbeManager(t)
	_ = m.AddModel(models.ModelConfig{
		Name:           "forced-xml",
		Provider:       "openai",
		Filename:       "some-model-id",
		ToolCallFormat: "xml",
		ProviderConfig: &models.ProviderConfig{BaseURL: server.URL, APIKey: "test-key"},
	})

	if got := m.EffectiveToolCallFormat(context.Background(), "forced-xml"); got != "xml" {
		t.Fatalf("expected explicit xml, got %q", got)
	}
	if n := probes.Load(); n != 0 {
		t.Fatalf("explicit override must not probe, got %d probes", n)
	}
}

// TestEffectiveToolCallFormat_ProbeFailureNotCached verifies a failed probe
// falls back to XML and is re-attempted on the next resolution.
func TestEffectiveToolCallFormat_ProbeFailureNotCached(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	m := newProbeManager(t)
	_ = m.AddModel(models.ModelConfig{
		Name:     "flaky-model",
		Provider: "openai",
		Filename: "some-model-id",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	})

	if got := m.EffectiveToolCallFormat(context.Background(), "flaky-model"); got != "" {
		t.Fatalf("expected XML fallback on probe failure, got %q", got)
	}
	if got := m.EffectiveToolCallFormat(context.Background(), "flaky-model"); got != "" {
		t.Fatalf("expected XML fallback on second probe failure, got %q", got)
	}
	if n := probes.Load(); n != 2 {
		t.Fatalf("failed probes must not be cached (expected 2 probes), got %d", n)
	}
}

// TestEffectiveToolCallFormat_UnknownModel returns empty for an unknown name.
func TestEffectiveToolCallFormat_UnknownModel(t *testing.T) {
	m := newProbeManager(t)
	if got := m.EffectiveToolCallFormat(context.Background(), "nope"); got != "" {
		t.Fatalf("expected empty for unknown model, got %q", got)
	}
}

// TestEffectiveToolCallFormat_LocalManagedModelAutoNative verifies that a
// manager-launched LOCAL llama.cpp model (provider "local") with an unset
// tool_call_format is auto-detected as native via the local probe — the same
// signal OpenAI-style registrations get — instead of always falling back to
// XML text mode. Regression: local models previously could never be probed
// (Build returned LocalProvider, not the OpenAI-compatible provider), so
// native-tool models mangled the XML format.
func TestEffectiveToolCallFormat_LocalManagedModelAutoNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/slots" {
			w.Write([]byte(`[{"id":0,"n_ctx":16384}]`))
			return
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
	}))
	defer server.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}

	m := newProbeManager(t)
	if err := m.AddModel(models.ModelConfig{
		Name:     "qwen-local",
		Provider: models.ProviderLocal,
		Filename: "Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Path:     "/models/Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Port:     port,
		Metadata: &models.ModelMetadata{Nctx: 262144, ContextLength: 262144},
	}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	if got := m.EffectiveToolCallFormat(context.Background(), "qwen-local"); got != "native" {
		t.Fatalf("EffectiveToolCallFormat = %q, want native (auto-detected for local llama.cpp)", got)
	}
	if cfg := findModel(t, m, "qwen-local"); cfg.ToolCallFormat != "native" {
		t.Errorf("tool_call_format not cached on model config: %q", cfg.ToolCallFormat)
	}
}
