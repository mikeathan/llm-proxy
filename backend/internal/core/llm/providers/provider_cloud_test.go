package providers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
)

func TestDynamicProviderRegistry_OpenRouter(t *testing.T) {
	registry := providers.GetRegistry()

	// Test that openrouter is loaded
	m, ok := registry.Get("openrouter")
	if !ok {
		t.Fatal("expected openrouter manifest to be loaded")
	}
	if m.Name != "OpenRouter" {
		t.Errorf("expected OpenRouter name, got %s", m.Name)
	}

	cfg := models.ModelConfig{
		Provider: "openrouter",
		ProviderConfig: &models.ProviderConfig{
			APIKey: "test-key",
		},
	}

	p := providers.NewOpenAICompatibleProvider(cfg, m)

	// Test GetEndpoint
	url, header, err := p.GetEndpoint(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://openrouter.ai/api/v1/chat/completions" {
		t.Errorf("expected URL https://openrouter.ai/api/v1/chat/completions, got %s", url)
	}
	if header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("expected Authorization header Bearer test-key, got %s", header.Get("Authorization"))
	}

	// Test ListModels with mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" || r.URL.Path == "/v1/slots" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/models" {
			t.Errorf("expected path /models, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model1"},{"id":"model2"}]}`))
	}))
	defer server.Close()

	// Override base URL for test
	cfg.ProviderConfig.BaseURL = server.URL
	p = providers.NewOpenAICompatibleProvider(cfg, m)

	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 || infos[0].ID != "model1" || infos[1].ID != "model2" {
		t.Errorf("unexpected models: %v", infos)
	}
}

func TestNvidiaProvider_Manifest(t *testing.T) {
	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey: "nvapi-test",
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	// Test GetEndpoint
	url, header, err := p.GetEndpoint(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://integrate.api.nvidia.com/v1/chat/completions" {
		t.Errorf("expected URL https://integrate.api.nvidia.com/v1/chat/completions, got %s", url)
	}
	if header.Get("Authorization") != "Bearer nvapi-test" {
		t.Errorf("expected Authorization header Bearer nvapi-test, got %s", header.Get("Authorization"))
	}
}

// TestProbeChatModel verifies the availability probe distinguishes a callable
// model (200), a listed-but-not-entitled model (404), and an unreachable
// upstream (connection closed before a response).
func TestProbeChatModel(t *testing.T) {
	newProvider := func(t *testing.T, server *httptest.Server) *providers.OpenAICompatibleProvider {
		t.Helper()
		m, _ := providers.GetRegistry().Get("nvidia")
		cfg := models.ModelConfig{
			Provider: "nvidia",
			ProviderConfig: &models.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}
		return providers.NewOpenAICompatibleProvider(cfg, m)
	}

	t.Run("available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"p"}}]}`))
		}))
		defer server.Close()

		p := newProvider(t, server)
		if err := p.ProbeChatModel(context.Background(), "deepseek-ai/deepseek-v4-flash-0731"); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})

	t.Run("not callable status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		p := newProvider(t, server)
		err := p.ProbeChatModel(context.Background(), "gpt-oss-120b")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "status 404") {
			t.Errorf("expected status 404 in error, got %v", err)
		}
	})

	t.Run("upstream unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		}))
		defer server.Close()

		p := newProvider(t, server)
		err := p.ProbeChatModel(context.Background(), "openai/gpt-oss-120b")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unreachable") {
			t.Errorf("expected unreachable in error, got %v", err)
		}
	})
}

// TestProbeNativeTools verifies the native-tool support probe distinguishes a
// server that returns tool_calls (finish_reason "tool_calls") from one that
// only returns plain text (no native tool support) and from upstream errors.
func TestProbeNativeTools(t *testing.T) {
	newProvider := func(t *testing.T, server *httptest.Server) *providers.OpenAICompatibleProvider {
		t.Helper()
		m, _ := providers.GetRegistry().Get("nvidia")
		cfg := models.ModelConfig{
			Provider: "nvidia",
			ProviderConfig: &models.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}
		return providers.NewOpenAICompatibleProvider(cfg, m)
	}

	t.Run("native tool calls supported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["tools"]; !ok {
				t.Error("probe request must include a tools schema")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
		}))
		defer server.Close()

		p := newProvider(t, server)
		supported, err := p.ProbeNativeTools(context.Background(), "some-model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !supported {
			t.Fatal("expected native tool support")
		}
	})

	t.Run("plain text only (no native tools)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"I cannot use tools."}}]}`))
		}))
		defer server.Close()

		p := newProvider(t, server)
		supported, err := p.ProbeNativeTools(context.Background(), "some-model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if supported {
			t.Fatal("expected no native tool support")
		}
	})

	t.Run("upstream error surfaces", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer server.Close()

		p := newProvider(t, server)
		if _, err := p.ProbeNativeTools(context.Background(), "some-model"); err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestProbeNativeTools_ThinkingModelRetry verifies the probe retries with a
// larger budget when the first attempt is truncated by the token cap before a
// tool call (finish_reason "length") — the signature of a thinking model that
// spends tokens on reasoning first. Without the retry, such models were
// misdetected as "no native support" and dropped into XML text mode.
func TestProbeNativeTools_ThinkingModelRetry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if requests.Load() == 1 {
			// First attempt: budget exhausted on reasoning, no tool call.
			w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","reasoning_content":"The user wants me to list the directory..."}}]}`))
			return
		}
		// Retry: thinking finished, tool call emitted.
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
	}))
	defer server.Close()

	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	supported, err := p.ProbeNativeTools(context.Background(), "thinking-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !supported {
		t.Fatal("expected native tool support after the length retry")
	}
	if requests.Load() != 2 {
		t.Errorf("expected exactly 2 probe requests (attempt + retry), got %d", requests.Load())
	}
}

// TestProbeNativeTools_LengthLimitedTwice verifies a model that stays
// length-limited on both attempts resolves to no native support.
func TestProbeNativeTools_LengthLimitedTwice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","reasoning_content":"thinking..."}}]}`))
	}))
	defer server.Close()

	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	supported, err := p.ProbeNativeTools(context.Background(), "deep-thinker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if supported {
		t.Fatal("expected no native tool support when length-limited twice")
	}
}

// TestProbeNativeTools_DeadlineEscalates verifies a slow server that exceeds
// the first attempt's deadline is given one escalation attempt (larger budget,
// longer deadline) before concluding — a slow-but-alive model must not be
// misclassified as "no native support" just because it generates slowly.
func TestProbeNativeTools_DeadlineEscalates(t *testing.T) {
	// Shrink the per-attempt deadline so the test runs in milliseconds; the
	// server responds slower than the first deadline but within the second.
	saved := providers.ProbeNativeToolTimeout
	providers.ProbeNativeToolTimeout = 50 * time.Millisecond
	t.Cleanup(func() { providers.ProbeNativeToolTimeout = saved })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
	}))
	defer server.Close()

	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	supported, err := p.ProbeNativeTools(ctx, "slow-model")
	if err != nil {
		t.Fatalf("escalated probe should succeed on a slow-but-alive server: %v", err)
	}
	if !supported {
		t.Fatal("expected native tool support after the deadline escalation")
	}
	if requests.Load() != 2 {
		t.Errorf("expected 2 probe requests (attempt + deadline escalation), got %d", requests.Load())
	}
}

// TestProbeNativeTools_DeadlineBothAttempts verifies that when BOTH attempts
// exceed their deadlines the probe fails with a deadline error rather than
// misreporting "no native support".
func TestProbeNativeTools_DeadlineBothAttempts(t *testing.T) {
	savedA := providers.ProbeNativeToolTimeout
	savedB := providers.ProbeNativeToolMaxTimeout
	providers.ProbeNativeToolTimeout = 50 * time.Millisecond
	providers.ProbeNativeToolMaxTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		providers.ProbeNativeToolTimeout = savedA
		providers.ProbeNativeToolMaxTimeout = savedB
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	supported, err := p.ProbeNativeTools(ctx, "glacial-model")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline error, got %v", err)
	}
	if supported {
		t.Fatal("expected no native support when both attempts time out")
	}
}

// TestProbeNativeTools_ConnectionErrorFailsFast verifies a transport error
// that is NOT a deadline (e.g. the server closes the connection) fails
// immediately without escalation — a dead endpoint must not stall the agent
// through the whole ladder.
func TestProbeNativeTools_ConnectionErrorFailsFast(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()

	m, _ := providers.GetRegistry().Get("nvidia")
	cfg := models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := providers.NewOpenAICompatibleProvider(cfg, m)

	if _, err := p.ProbeNativeTools(context.Background(), "dead-model"); err == nil {
		t.Fatal("expected an error for a connection-reset probe")
	}
	if requests.Load() != 1 {
		t.Errorf("expected exactly 1 probe request (no escalation on transport error), got %d", requests.Load())
	}
}
