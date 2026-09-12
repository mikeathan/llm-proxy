package searchproviders

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

func TestTavilyProvider_Search_Success(t *testing.T) {
	var method, contentType, path, auth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[
			{"title":"A","url":"https://a.example","content":"sa"},
			{"title":"B","url":"https://b.example","content":"sb"}
		]}`)
	}))
	defer srv.Close()

	p, err := newTavilyProvider(tools.SearchProviderConfig{APIKey: "secret-key", Client: newTestClient(srv), MaxResults: 5})
	if err != nil {
		t.Fatalf("newTavilyProvider() error = %v", err)
	}

	results, err := p.Search(context.Background(), "golang generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if path != "/search" {
		t.Errorf("path = %q, want /search", path)
	}
	// Tavily authenticates via the Authorization header; the key must not ride
	// in the JSON body (the legacy field is rejected with 401).
	if auth != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer secret-key")
	}
	if _, ok := body["api_key"]; ok {
		t.Errorf("body must not carry api_key, got %v", body)
	}
	if body["query"] != "golang generics" {
		t.Errorf("query = %v, want %q", body["query"], "golang generics")
	}
	if body["search_depth"] != "basic" {
		t.Errorf("search_depth = %v, want basic", body["search_depth"])
	}
	if body["max_results"] != float64(5) {
		t.Errorf("max_results = %v, want 5", body["max_results"])
	}
	if len(results) != 2 {
		t.Fatalf("results = %v, want 2", results)
	}
	if results[0] != (tools.SearchResult{Title: "A", URL: "https://a.example", Snippet: "sa"}) {
		t.Errorf("results[0] = %+v", results[0])
	}
}

func TestTavilyProvider_Search_DefaultsMaxResults(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	p, err := newTavilyProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv), MaxResults: 0})
	if err != nil {
		t.Fatalf("newTavilyProvider() error = %v", err)
	}
	if _, err := p.Search(context.Background(), "q"); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if body["max_results"] != float64(5) {
		t.Errorf("max_results = %v, want default 5", body["max_results"])
	}
}

func TestTavilyProvider_Search_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"detail":"unauthorized"}`)
	}))
	defer srv.Close()

	p, err := newTavilyProvider(tools.SearchProviderConfig{APIKey: "top-secret", Client: newTestClient(srv)})
	if err != nil {
		t.Fatalf("newTavilyProvider() error = %v", err)
	}
	_, err = p.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "status 401") || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("err = %v, want status and body detail", err)
	}
	if !errors.Is(err, models.ErrToolUnavailable) {
		t.Errorf("401 must be classified terminal (models.ErrToolUnavailable), got %v", err)
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Errorf("err leaked the API key: %v", err)
	}
}

func TestTavilyProvider_Search_SkipsMalformedURLsAndCapsResults(t *testing.T) {
	results := []map[string]string{
		{"title": "bad", "url": "not-a-url", "content": "x"},
		{"title": "ok1", "url": "https://one.example", "content": "1"},
		{"title": "rel", "url": "/relative", "content": "2"},
		{"title": "ok2", "url": "http://two.example", "content": "3"},
		{"title": "ok3", "url": "https://three.example", "content": "4"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer srv.Close()

	p, err := newTavilyProvider(tools.SearchProviderConfig{APIKey: "k", Client: newTestClient(srv), MaxResults: 2})
	if err != nil {
		t.Fatalf("newTavilyProvider() error = %v", err)
	}
	got, err := p.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("results = %v, want 2 (capped)", got)
	}
	if got[0].URL != "https://one.example" || got[1].URL != "http://two.example" {
		t.Errorf("results = %+v, want the two valid URLs", got)
	}
}

func TestNewTavilyProvider_RejectsBadConfig(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		if _, err := newTavilyProvider(tools.SearchProviderConfig{APIKey: "k"}); err == nil {
			t.Fatal("expected an error for a nil client")
		}
	})
	t.Run("missing key", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		if _, err := newTavilyProvider(tools.SearchProviderConfig{Client: newTestClient(srv)}); err == nil {
			t.Fatal("expected an error for a missing API key")
		}
	})
}
