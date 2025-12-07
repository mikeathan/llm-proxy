package proxy_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
)

type mockProxy struct {
	called bool
}

func (m *mockProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.called = true
	w.WriteHeader(200)
	w.Write([]byte("proxied"))
}

func TestChatHandler_MissingHeader(t *testing.T) {
	srv := proxy.NewServer(nil) // Mgr not used for this case

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	srv.ChatHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing X-Model-Name") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandler_ModelStarting(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(name string) (proxy.ModelInstance, error) {
			return proxy.ModelInstance{}, proxy.ErrModelStarting
		},
	}

	srv := proxy.NewServer(mgr)

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	srv.ChatHandler(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After missing")
	}
	if w.Header().Get("X-LLM-Status") != "starting" {
		t.Fatalf("X-LLM-Status missing")
	}
	if w.Body.String() != `{"status":"starting"}` {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandler_ModelError(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(name string) (proxy.ModelInstance, error) {
			return proxy.ModelInstance{}, errors.New("boom")
		},
	}

	srv := proxy.NewServer(mgr)

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	srv.ChatHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "model error") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandler_ProxyCalled(t *testing.T) {
	// mock reverse proxy
	mp := &mockProxy{}

	restore := proxy.SetReverseProxyFactory(func(target string) http.Handler {
		return mp
	})
	defer restore()

	mgr := &mocks.MockManager{
		EnsureModelFunc: func(name string) (proxy.ModelInstance, error) {
			return proxy.ModelInstance{Port: 9999}, nil
		},
		RecordActivityFunc: func(name string) {},
	}

	srv := proxy.NewServer(mgr)

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	srv.ChatHandler(w, req)

	if !mp.called {
		t.Fatalf("expected reverse proxy ServeHTTP to be called")
	}

	if w.Code != 200 || w.Body.String() != "proxied" {
		t.Fatalf("unexpected proxy output: %d %s", w.Code, w.Body.String())
	}
}
