package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

func newToolSecretDeleteHandler(store *mocks.MockSecretsStore) *handlers.SecretsHandlers {
	return handlers.NewSecretsHandlers(&mocks.MockAdminService{
		SecretsFunc: func() models.SecretsStore { return store },
	})
}

func TestAdminToolSecretDeleteHandler_ClearsSecret(t *testing.T) {
	var gotCategory, gotProvider string
	store := &mocks.MockSecretsStore{
		DeleteSecretFunc: func(category, provider string) error {
			gotCategory, gotProvider = category, provider
			return nil
		},
		// After a successful delete the store reports no secret.
		MaskedSecretFunc: func(category, provider string) string { return "" },
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/secrets/tools?category=search&provider=tavily", nil)
	rec := httptest.NewRecorder()
	newToolSecretDeleteHandler(store).AdminToolSecretDeleteHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotCategory != "search" || gotProvider != "tavily" {
		t.Errorf("store called with (%q, %q), want (search, tavily)", gotCategory, gotProvider)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["secret"] != "" {
		t.Errorf("secret = %q, want empty after delete", body["secret"])
	}
}

// An empty secret is not a credential: accepting it would silently wipe a
// working key (a stray or malformed PUT would clear the provider). Clearing is
// DELETE's job.
func TestAdminToolSecretPutHandler_RejectsEmptySecret(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty string", `{"secret":""}`},
		{"whitespace only", `{"secret":"   "}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			store := &mocks.MockSecretsStore{
				SetSecretFunc: func(category, provider, value string) error {
					called = true
					return nil
				},
			}

			req := httptest.NewRequest(http.MethodPut, "/admin/api/secrets/tools?category=search&provider=tavily", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newToolSecretDeleteHandler(store).AdminToolSecretPutHandler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if called {
				t.Error("an empty secret must not reach the store")
			}
		})
	}
}

// A malformed request must not reach the store: a missing parameter would
// otherwise delete the wrong (or an empty-category) secret.
func TestAdminToolSecretDeleteHandler_RejectsMissingParams(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"missing provider", "?category=search"},
		{"missing category", "?provider=tavily"},
		{"missing both", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			store := &mocks.MockSecretsStore{
				DeleteSecretFunc: func(category, provider string) error {
					called = true
					return nil
				},
			}

			req := httptest.NewRequest(http.MethodDelete, "/admin/api/secrets/tools"+tc.query, nil)
			rec := httptest.NewRecorder()
			newToolSecretDeleteHandler(store).AdminToolSecretDeleteHandler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if called {
				t.Error("store must not be called when a parameter is missing")
			}
		})
	}
}
