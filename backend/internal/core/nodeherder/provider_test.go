package nodeherder_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"llm-proxy/internal/core/nodeherder"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// ServiceTokenManager Tests
func TestServiceTokenManager_SuccessAndCaches(t *testing.T) {
	var calls int32
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if r.URL.Path != "/api/auth/token" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("expected content-type application/json")
			}

			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if payload["client_id"] != "client-id" {
				t.Fatalf("unexpected client_id: %s", payload["client_id"])
			}
			if payload["client_secret"] != "client-secret" {
				t.Fatalf("unexpected client_secret: %s", payload["client_secret"])
			}

			return newTestResponse(http.StatusOK, `{"access_token":"token-1","expires_in":120}`), nil
		}),
	}

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(client, "http://example.test")

	token1, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 != "token-1" || token2 != "token-1" {
		t.Fatalf("unexpected tokens: %s, %s", token1, token2)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 token request, got %d", calls)
	}
}

func TestServiceTokenManager_ShortExpiryForcesRefresh(t *testing.T) {

	var calls int32
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			call := atomic.AddInt32(&calls, 1)
			token := "token-1"
			if call == 2 {
				token = "token-2"
			}
			return newTestResponse(http.StatusOK, `{"access_token":"`+token+`","expires_in":30}`), nil
		}),
	}

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(client, "http://example.test")

	token1, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 == token2 {
		t.Fatalf("expected refreshed token, got %s", token2)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 token requests, got %d", calls)
	}
}

func TestServiceTokenManager_Non200Response(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusInternalServerError, "boom"), nil
		}),
	}

	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	manager := nodeherder.NewServiceTokenManager(client, "http://example.test")

	_, err := manager.Get(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "token request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// SanitiseUrl Test
func TestSanitiseUrl(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/http://example.com", "http://example.com"},
		{"http://example.com", "http://example.com"},
	}

	for _, tt := range tests {
		got := sanitiseUrl(tt.in)
		if got != tt.want {
			t.Fatalf("sanitiseUrl(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func sanitiseUrl(url string) string {
	if after, ok := strings.CutPrefix(url, "/"); ok {
		return after
	}
	return url
}
