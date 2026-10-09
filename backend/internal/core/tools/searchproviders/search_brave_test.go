package searchproviders

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

func TestBraveProvider_Search_Success(t *testing.T) {
	var method, path, token string
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		token = r.Header.Get(braveTokenHdr)
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"web":{"results":[
			{"title":"Go","url":"https://go.dev","description":"The Go language"},
			{"title":"Blog","url":"https://go.dev/blog","description":"Go blog"}
		]}}`)
	}))
	defer srv.Close()

	p, err := newBraveProvider(tools.SearchProviderConfig{APIKey: "brave-key", Client: newTestClient(srv), MaxResults: 7})
	if err != nil {
		t.Fatalf("newBraveProvider() error = %v", err)
	}
	results, err := p.Search(context.Background(), "go generics", "")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
	if path != "/res/v1/web/search" {
		t.Errorf("path = %q, want /res/v1/web/search", path)
	}
	if token != "brave-key" {
		t.Errorf("token header = %q, want brave-key", token)
	}
	if got := query.Get("q"); got != "go generics" {
		t.Errorf("q = %q, want %q", got, "go generics")
	}
	if got := query.Get("count"); got != "7" {
		t.Errorf("count = %q, want 7", got)
	}
	if len(results) != 2 || results[0].URL != "https://go.dev" || results[0].Snippet != "The Go language" {
		t.Errorf("results = %+v", results)
	}
}

func TestBraveProvider_Search_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "invalid token")
	}))
	defer srv.Close()

	p, err := newBraveProvider(tools.SearchProviderConfig{APIKey: "sensitive", Client: newTestClient(srv)})
	if err != nil {
		t.Fatalf("newBraveProvider() error = %v", err)
	}
	_, err = p.Search(context.Background(), "q", "")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Errorf("err = %v, want status 403", err)
	}
	if !errors.Is(err, models.ErrToolUnavailable) {
		t.Errorf("403 must be classified terminal (models.ErrToolUnavailable), got %v", err)
	}
	if strings.Contains(err.Error(), "sensitive") {
		t.Errorf("err leaked the API key: %v", err)
	}
}

// Brave rejects a bad API key with 422 (not 401/403) and the code SUBSCRIPTION_TOKEN_INVALID — the body below is
// Brave's real response (captured 2026-10-09). It must be classified as an unavailable tool, or the agent keeps
// going and invents an answer. A 422 with any other code is an input problem and stays a plain error.
func TestBraveProvider_Search_InvalidKey422IsUnavailable(t *testing.T) {
	const invalidKey = `{"type":"ErrorResponse","error":{"id":"3f0c623e","status":422,"detail":"The provided API key is invalid.","meta":{"component":"authentication"},"code":"SUBSCRIPTION_TOKEN_INVALID"},"time":1791545452}`
	const badQuery = `{"type":"ErrorResponse","error":{"id":"9a1b","status":422,"detail":"Unable to validate request parameter(s)","code":"VALIDATION"}}`
	for _, tc := range []struct {
		name, body  string
		unavailable bool
	}{
		{"invalid subscription token", invalidKey, true},
		{"other 422", badQuery, false},
		{"422 without a JSON body", "unprocessable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			p, err := newBraveProvider(tools.SearchProviderConfig{APIKey: "sensitive", Client: newTestClient(srv)})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Search(context.Background(), "q", "")
			if err == nil || !strings.Contains(err.Error(), "status 422") {
				t.Fatalf("err = %v, want a status 422 error", err)
			}
			if got := errors.Is(err, models.ErrToolUnavailable); got != tc.unavailable {
				t.Errorf("unavailable = %v, want %v (err %v)", got, tc.unavailable, err)
			}
		})
	}
}

func TestBraveProvider_Search_SkipsMalformedAndCaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"web":{"results":[
			{"title":"bad","url":"not-a-url","description":"x"},
			{"title":"one","url":"https://one.example","description":"1"},
			{"title":"two","url":"https://two.example","description":"2"},
			{"title":"three","url":"https://three.example","description":"3"}
		]}}`)
	}))
	defer srv.Close()

	p, err := newBraveProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv), MaxResults: 2})
	if err != nil {
		t.Fatalf("newBraveProvider() error = %v", err)
	}
	got, err := p.Search(context.Background(), "q", "")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 2 || got[0].URL != "https://one.example" || got[1].URL != "https://two.example" {
		t.Errorf("results = %+v, want the two valid capped URLs", got)
	}
}

func TestNewBraveProvider_RejectsBadConfig(t *testing.T) {
	if _, err := newBraveProvider(tools.SearchProviderConfig{APIKey: "k"}); err == nil {
		t.Error("expected an error for a nil client")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if _, err := newBraveProvider(tools.SearchProviderConfig{Client: newTestClient(srv)}); err == nil {
		t.Error("expected an error for a missing API key")
	}
}
