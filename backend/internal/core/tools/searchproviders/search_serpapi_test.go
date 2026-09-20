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

func TestSerpAPIProvider_Search_Success(t *testing.T) {
	var method, path string
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		query = r.URL.Query()
		io.WriteString(w, `{"organic_results":[
			{"title":"Go","link":"https://go.dev","snippet":"The Go language"},
			{"title":"Blog","link":"https://go.dev/blog","snippet":"Go blog"}
		]}`)
	}))
	defer srv.Close()

	p, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "serp-key", Client: newTestClient(srv), MaxResults: 8})
	if err != nil {
		t.Fatalf("newSerpAPIProvider() error = %v", err)
	}
	results, err := p.Search(context.Background(), "go generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
	if path != "/search.json" {
		t.Errorf("path = %q, want /search.json", path)
	}
	if got := query.Get("engine"); got != "google" {
		t.Errorf("engine = %q, want google", got)
	}
	if got := query.Get("q"); got != "go generics" {
		t.Errorf("q = %q, want %q", got, "go generics")
	}
	if got := query.Get("api_key"); got != "serp-key" {
		t.Errorf("api_key = %q, want serp-key", got)
	}
	if got := query.Get("num"); got != "8" {
		t.Errorf("num = %q, want 8", got)
	}
	if len(results) != 2 || results[0].URL != "https://go.dev" || results[0].Snippet != "The Go language" {
		t.Errorf("results = %+v", results)
	}
}

func TestSerpAPIProvider_Search_ErrorFieldWith200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"error":"Invalid API key"}`)
	}))
	defer srv.Close()

	p, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv)})
	if err != nil {
		t.Fatalf("newSerpAPIProvider() error = %v", err)
	}
	_, err = p.Search(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("err = %v, want the API error field surfaced", err)
	}
}

func TestSerpAPIProvider_Search_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, "rate limited")
	}))
	defer srv.Close()

	p, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "secret-api-key", Client: newTestClient(srv)})
	if err != nil {
		t.Fatalf("newSerpAPIProvider() error = %v", err)
	}
	_, err = p.Search(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("err = %v, want status 429", err)
	}
	if errors.Is(err, models.ErrToolUnavailable) {
		t.Error("429 (rate limit) is transient and must NOT be classified terminal")
	}
	if strings.Contains(err.Error(), "secret-api-key") {
		t.Errorf("err leaked the API key: %v", err)
	}
}

func TestSerpAPIProvider_Search_AuthStatusClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "invalid api key")
	}))
	defer srv.Close()

	p, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv)})
	if err != nil {
		t.Fatalf("newSerpAPIProvider() error = %v", err)
	}
	_, err = p.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !errors.Is(err, models.ErrToolUnavailable) {
		t.Fatalf("401 must be classified terminal (models.ErrToolUnavailable), got %v", err)
	}
}

func TestSerpAPIProvider_Search_SkipsMalformedAndCaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"organic_results":[
			{"title":"bad","link":"javascript:alert(1)","snippet":"x"},
			{"title":"one","link":"https://one.example","snippet":"1"},
			{"title":"two","link":"https://two.example","snippet":"2"},
			{"title":"three","link":"https://three.example","snippet":"3"}
		]}`)
	}))
	defer srv.Close()

	p, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv), MaxResults: 2})
	if err != nil {
		t.Fatalf("newSerpAPIProvider() error = %v", err)
	}
	got, err := p.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 2 || got[0].URL != "https://one.example" || got[1].URL != "https://two.example" {
		t.Errorf("results = %+v, want the two valid capped URLs", got)
	}
}

func TestNewSerpAPIProvider_RejectsBadConfig(t *testing.T) {
	if _, err := newSerpAPIProvider(tools.SearchProviderConfig{APIKey: "k"}); err == nil {
		t.Error("expected an error for a nil client")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if _, err := newSerpAPIProvider(tools.SearchProviderConfig{Client: newTestClient(srv)}); err == nil {
		t.Error("expected an error for a missing API key")
	}
}
