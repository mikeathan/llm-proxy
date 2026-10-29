package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

// E5 (docs/PLANS/cross-cutting/admin-api-authentication-design.md), run and confirmed 2026-10-29: the registrar builds a
// provider from a stored key's name plus a base_url it is given, and the connection test sends the real key to that URL.
// That is a real capability (a custom OpenAI-compatible endpoint is a base_url with a stored key), so it is not removed
// here; the protection is who may ask. The admin endpoint that reached this with query-string values on a GET is a POST
// with a body and behind the cross-site gate (transport/http guard), and a stored key with a custom base_url asks for the
// passphrase again once sign-in exists (plan, step-up). This test pins the behaviour so a change to it is deliberate.
func TestE5_AStoredKeyGoesToTheBaseURLItIsBuiltWith(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer attacker.Close()

	secrets := &mocks.MockSecretsStore{
		GetResolvedProviderKeyInfoFunc: func(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
			return &models.ResolvedProviderKeyInfo{Key: "nvapi-real-key"}, nil // a stored key with no base_url of its own
		},
	}
	r := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, "127.0.0.1")
	// The app injects its provider client here; a plain client stands in for it, as an attacker's server is a public one.
	r.SetHTTPDoer(http.DefaultClient)
	p, err := r.Build(models.ModelConfig{
		Provider:       "nvidia",
		ProviderConfig: &models.ProviderConfig{APIKeyName: "nvidia-build", BaseURL: attacker.URL},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_ = p.TestConnection(context.Background())

	mu.Lock()
	defer mu.Unlock()
	t.Logf("requests seen by the caller-chosen URL: %v", seen)
	if len(seen) == 0 {
		t.Fatal("no request reached the base URL")
	}
	if seen[0] != "Bearer nvapi-real-key" {
		t.Errorf("the stored key no longer goes to the base URL it was built with (%q): update the plan's E5 note", seen[0])
	}
}
