package providers_test

import (
	"errors"
	"testing"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

// TestBuildFailsLoudOnUnresolvableNamedKey verifies that a cloud model which
// explicitly references a credential by name that cannot be hydrated returns
// ErrUnresolvedKey at Build time — instead of silently shipping an empty
// Authorization header that would only surface as a confusing 401 later.
func TestBuildFailsLoudOnUnresolvableNamedKey(t *testing.T) {
	secrets := &mocks.MockSecretsStore{
		GetResolvedProviderKeyInfoFunc: func(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
			return nil, errors.New("key not found")
		},
		ResolveMaskedKeyFunc: func(provider, maskedKey string) (string, error) {
			return "", errors.New("no key matching mask")
		},
	}
	r := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, "127.0.0.1")

	_, err := r.Build(models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKeyName: "nvidia-build",
		},
	})
	if err == nil {
		t.Fatal("expected error for unresolvable named key, got nil")
	}
	if !errors.Is(err, providers.ErrUnresolvedKey) {
		t.Fatalf("expected ErrUnresolvedKey, got %v", err)
	}
}

// TestBuildFailsLoudOnUnresolvableMaskedKey verifies that a masked placeholder
// key that cannot be hydrated back to a real credential fails fast.
func TestBuildFailsLoudOnUnresolvableMaskedKey(t *testing.T) {
	secrets := &mocks.MockSecretsStore{
		GetResolvedProviderKeyInfoFunc: func(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
			return nil, errors.New("key not found")
		},
		ResolveMaskedKeyFunc: func(provider, maskedKey string) (string, error) {
			return "", errors.New("no key matching mask")
		},
	}
	r := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, "127.0.0.1")

	_, err := r.Build(models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKey: "nvapi-...1234",
		},
	})
	if err == nil {
		t.Fatal("expected error for unresolvable masked key, got nil")
	}
	if !errors.Is(err, providers.ErrUnresolvedKey) {
		t.Fatalf("expected ErrUnresolvedKey, got %v", err)
	}
}

// TestBuildResolvesNamedKey verifies the happy path: a named credential that
// resolves injects the real key and per-key base URL.
func TestBuildResolvesNamedKey(t *testing.T) {
	secrets := &mocks.MockSecretsStore{
		GetResolvedProviderKeyInfoFunc: func(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
			if name != "nvidia-build" {
				return nil, errors.New("unexpected name")
			}
			return &models.ResolvedProviderKeyInfo{
				Key:     "nvapi-real-key",
				BaseURL: "https://custom.example.com/v1",
			}, nil
		},
	}
	r := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, "127.0.0.1")

	p, err := r.Build(models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{
			APIKeyName: "nvidia-build",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

// TestBuildToleratesUnkeyedModel verifies that a cloud model with no named
// credential and nothing stored still builds (a no-auth endpoint is tolerated)
// rather than being rejected.
func TestBuildToleratesUnkeyedModel(t *testing.T) {
	secrets := &mocks.MockSecretsStore{
		GetResolvedProviderKeyInfoFunc: func(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
			return nil, errors.New("no keys")
		},
	}
	r := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, "127.0.0.1")

	p, err := r.Build(models.ModelConfig{
		Provider: "nvidia",
		ProviderConfig: &models.ProviderConfig{},
	})
	if err != nil {
		t.Fatalf("expected unkeyed model to build, got error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}
