package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/testing/mocks"
)

// The connection test used to be a GET that took the key, the key's name and the base URL from the query string
// (plan admin-api-authentication, E5/E6): a cross-site <img> could fire it, and the secrets ended up in logs and history.
// It is a POST with a JSON body.
func TestAdminTestProviderConnection_ReadsTheBodyNotTheQuery(t *testing.T) {
	var gotProvider, gotKey, gotName, gotBase string
	runtime := &mocks.MockManager{
		TestProviderConnectionFunc: func(_ context.Context, provider, apiKey, apiKeyName, baseURL string) error {
			gotProvider, gotKey, gotName, gotBase = provider, apiKey, apiKeyName, baseURL
			return nil
		},
	}
	h := NewModelHandlers(runtime, &mocks.MockAdminService{})

	body := `{"provider":"nvidia","api_key":"k","api_key_name":"build","base_url":"https://api.example.com/v1"}`
	if rec := do(h.AdminTestProviderConnectionHandler, http.MethodPost, "/admin/api/providers/test", body); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
	}
	if gotProvider != "nvidia" || gotKey != "k" || gotName != "build" || gotBase != "https://api.example.com/v1" {
		t.Errorf("got %q %q %q %q", gotProvider, gotKey, gotName, gotBase)
	}

	// Values in the query string are ignored: nothing a link can carry reaches the provider.
	gotProvider = ""
	rec := do(h.AdminTestProviderConnectionHandler, http.MethodPost, "/admin/api/providers/test?provider=nvidia&api_key_name=build&base_url=http://attacker", `{}`)
	if rec.Code != http.StatusBadRequest || gotProvider != "" {
		t.Errorf("a body with no provider = %d (provider %q), want 400 and no call", rec.Code, gotProvider)
	}
}

func TestAdminTestProviderConnection_FailureIsReported(t *testing.T) {
	runtime := &mocks.MockManager{TestProviderConnectionFunc: func(context.Context, string, string, string, string) error {
		return context.DeadlineExceeded
	}}
	h := NewModelHandlers(runtime, &mocks.MockAdminService{})
	rec := do(h.AdminTestProviderConnectionHandler, http.MethodPost, "/", `{"provider":"nvidia"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "connection test failed") {
		t.Errorf("= %d %s", rec.Code, rec.Body.String())
	}
}
