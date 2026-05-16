package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
)

func TestDynamicProviderRegistry(t *testing.T) {
	registry := providers.GetRegistry()
	
	// Test that mulerouter is loaded
	m, ok := registry.Get("mulerouter")
	if !ok {
		t.Fatal("expected mulerouter manifest to be loaded")
	}
	if m.Name != "MuleRouter" {
		t.Errorf("expected MuleRouter name, got %s", m.Name)
	}

	cfg := models.ModelConfig{
		Provider: "mulerouter",
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
	if url != "https://api.mulerouter.ai/vendors/openai/v1/chat/completions" {
		t.Errorf("expected URL https://api.mulerouter.ai/vendors/openai/v1/chat/completions, got %s", url)
	}
	if header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("expected Authorization header Bearer test-key, got %s", header.Get("Authorization"))
	}

	// Test ListModels with mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
