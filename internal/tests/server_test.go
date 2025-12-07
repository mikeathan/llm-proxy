package proxy_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
	"llm-proxy/models"
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

func TestAdminStateHandler(t *testing.T) {
	now := time.Now()
	mgr := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{
				{Name: "alpha", Path: "/models/alpha.gguf", Port: 8081},
				{Name: "beta", Path: "/models/beta.gguf", Port: 8082},
			}
		},
		ActiveInfoFunc: func() *proxy.ActiveModelInfo {
			return &proxy.ActiveModelInfo{
				Name:     "alpha",
				Port:     8081,
				Host:     "127.0.0.1",
				Ready:    true,
				Started:  now,
				LastUsed: now,
			}
		},
		ModelHostFunc: func() string { return "127.0.0.1" },
	}

	srv := proxy.NewServer(mgr)
	req := httptest.NewRequest("GET", "/admin/api/state", nil)
	w := httptest.NewRecorder()

	srv.AdminStateHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Models []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Ready  bool   `json:"ready"`
		} `json:"models"`
		Active struct {
			Name string `json:"name"`
		} `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}
	if resp.Active.Name != "alpha" {
		t.Fatalf("expected active model alpha, got %s", resp.Active.Name)
	}
	var activeRow struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
		Ready  bool   `json:"ready"`
	}
	for _, m := range resp.Models {
		if m.Name == "alpha" {
			activeRow = m
		}
	}
	if !activeRow.Active || !activeRow.Ready {
		t.Fatalf("alpha should be active and ready")
	}
}

func TestAdminStartHandler(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(name string) (proxy.ModelInstance, error) {
			return proxy.ModelInstance{Name: name, Port: 9090}, nil
		},
		RecordActivityFunc: func(name string) {},
		ModelHostFunc:      func() string { return "127.0.0.1" },
	}

	srv := proxy.NewServer(mgr)
	req := httptest.NewRequest("POST", "/admin/api/start", strings.NewReader(`{"name":"gamma"}`))
	w := httptest.NewRecorder()

	srv.AdminStartHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"gamma"`) {
		t.Fatalf("response should mention model name, got %s", w.Body.String())
	}
}

func TestAdminStopHandler(t *testing.T) {
	stopped := false
	mgr := &mocks.MockManager{
		ActiveInfoFunc: func() *proxy.ActiveModelInfo { return &proxy.ActiveModelInfo{Name: "delta"} },
		StopActiveFunc: func() error {
			stopped = true
			return nil
		},
	}

	srv := proxy.NewServer(mgr)
	req := httptest.NewRequest("POST", "/admin/api/stop", nil)
	w := httptest.NewRecorder()

	srv.AdminStopHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !stopped {
		t.Fatalf("StopActive should be called")
	}
	if !strings.Contains(w.Body.String(), "delta") {
		t.Fatalf("expected response to include stopped model, got %s", w.Body.String())
	}
}
