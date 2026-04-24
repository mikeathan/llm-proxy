package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter_MethodMatch(t *testing.T) {
	router := NewRouter()
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestRouter_MethodNotAllowed_Default(t *testing.T) {
	router := NewRouter()
	router.Get("/only-get", func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodPost, "/only-get", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	if w.Body.String() != "method not allowed\n" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestRouter_MethodNotAllowed_Custom(t *testing.T) {
	router := NewRouter()
	router.Post("/only-post", func(w http.ResponseWriter, r *http.Request) {},
		WithMethodNotAllowed(http.HandlerFunc(MethodNotAllowedJSON)),
	)

	req := httptest.NewRequest(http.MethodGet, "/only-post", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	if w.Body.String() != "{\"error\":\"method not allowed\"}\n" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected Content-Type: %s", ct)
	}
}

func TestRouter_AnyFallback(t *testing.T) {
	router := NewRouter()
	router.Any("/any", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/any", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestRouter_MethodOverridesAny(t *testing.T) {
	router := NewRouter()
	router.Any("/mixed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	router.Get("/mixed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/mixed", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDecodeJSON_BodyLimit(t *testing.T) {
	// 1. Create a large body (> 4MB)
	largeBody := strings.Repeat("a", 5*1024*1024)
	r := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(`{"message":"`+largeBody+`"}`)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var payload struct {
		Message string `json:"message"`
	}

	// 2. Call decodeJSON
	err := decodeJSON(w, r, &payload)

	// 3. Verify it failed due to size limit
	if err == nil {
		t.Errorf("expected error for body exceeding 4MB limit, got nil")
	}

	// http.MaxBytesReader returns a specific error type that json.Decoder wraps
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestDecodeJSON_ValidBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(`{"message":"hello"}`)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var payload struct {
		Message string `json:"message"`
	}

	err := decodeJSON(w, r, &payload)
	if err != nil {
		t.Errorf("expected no error for valid 1MB body, got: %v", err)
	}

	if payload.Message != "hello" {
		t.Errorf("expected message 'hello', got: %s", payload.Message)
	}
}
