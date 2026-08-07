package providers_test

import (
	"errors"
	"testing"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
)

// TestUnknownProviderFailsClosed verifies that a model referencing an unknown
// provider returns a typed error instead of silently falling back to the local
// host.  Removing a provider (vertex/mulerouter deprecation) must surface
// affected models, never re-route them to the local host.
func TestUnknownProviderFailsClosed(t *testing.T) {
	r := providers.NewProviderRegistrar(providers.GetRegistry(), nil, "127.0.0.1")
	r.RegisterLocal("llama-server", "", nil)

	_, err := r.Build(models.ModelConfig{
		Provider: "vertex",
		ProviderConfig: &models.ProviderConfig{
			APIKey: "test",
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider, got %v", err)
	}

	// A registered provider still builds.
	p, err := r.Build(models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey: "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error for nvidia: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider for nvidia")
	}
}
