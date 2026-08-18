package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
