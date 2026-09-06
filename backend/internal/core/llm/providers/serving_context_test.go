package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"llm-proxy/models"
)

// localLlamaServer serves the metadata endpoints of a llama.cpp instance:
// /slots (authoritative n_ctx) and /v1/models (n_ctx + n_ctx_train).
func localLlamaServer(t *testing.T, slots bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case slots && r.URL.Path == "/slots":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":0,"n_ctx":16384}]`))
		case r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"object":"list","data":[{"id":"Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf","object":"model","owned_by":"llamacpp","meta":{"n_ctx":16384,"n_ctx_train":262144}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestLocalProviderServingContext(t *testing.T) {
	server := localLlamaServer(t, true)
	defer server.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}

	p := NewLocalProvider(modelConfigWithPort(port), "", "", "127.0.0.1")
	if got := p.ServingContext(context.Background()); got != 16384 {
		t.Errorf("ServingContext = %d, want 16384 (from /slots)", got)
	}
}

func TestLocalProviderServingContext_ModelsFallback(t *testing.T) {
	// No /slots endpoint — the /v1/models meta.n_ctx must be used (and win
	// over n_ctx_train).
	server := localLlamaServer(t, false)
	defer server.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}

	p := NewLocalProvider(modelConfigWithPort(port), "", "", "127.0.0.1")
	if got := p.ServingContext(context.Background()); got != 16384 {
		t.Errorf("ServingContext = %d, want 16384 (from /v1/models n_ctx)", got)
	}
}

func TestLocalProviderServingContext_Unreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	url := server.URL
	server.Close() // server is down — probe must fail fast and return 0

	port, err := strconv.Atoi(strings.TrimPrefix(url, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}
	p := NewLocalProvider(modelConfigWithPort(port), "", "", "127.0.0.1")
	if got := p.ServingContext(context.Background()); got != 0 {
		t.Errorf("ServingContext = %d, want 0 for unreachable server", got)
	}
}

func TestLocalProviderServingContext_NoPort(t *testing.T) {
	p := NewLocalProvider(models.ModelConfig{}, "", "", "127.0.0.1")
	if got := p.ServingContext(context.Background()); got != 0 {
		t.Errorf("ServingContext = %d, want 0 without a configured port", got)
	}
}

// modelConfigWithPort builds a minimal local model config bound to port.
func modelConfigWithPort(port int) models.ModelConfig {
	return models.ModelConfig{
		Name:     "qwen",
		Provider: models.ProviderLocal,
		Filename: "Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Path:     "/models/Qwen_Qwen3.5-35B-A3B-Q4_K_M.gguf",
		Port:     port,
	}
}

// probeChatServer answers /v1/chat/completions like a native-tool llama.cpp
// when native=true, otherwise like a plain chat model.
func probeChatServer(t *testing.T, native bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if native {
			w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"I cannot call tools."}}]}`))
	}))
}

func TestLocalProviderProbeNativeTools(t *testing.T) {
	t.Run("native server detected", func(t *testing.T) {
		server := probeChatServer(t, true)
		defer server.Close()
		p := NewLocalProvider(modelConfigWithPort(portOf(t, server.URL)), "", "", "127.0.0.1")
		ok, err := p.ProbeNativeTools(context.Background(), "m.gguf")
		if err != nil || !ok {
			t.Fatalf("ProbeNativeTools = (%v, %v), want (true, nil)", ok, err)
		}
	})
	t.Run("plain chat server not native", func(t *testing.T) {
		server := probeChatServer(t, false)
		defer server.Close()
		p := NewLocalProvider(modelConfigWithPort(portOf(t, server.URL)), "", "", "127.0.0.1")
		ok, err := p.ProbeNativeTools(context.Background(), "m.gguf")
		if err != nil || ok {
			t.Fatalf("ProbeNativeTools = (%v, %v), want (false, nil)", ok, err)
		}
	})
	t.Run("unreachable server errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := server.URL
		server.Close()
		p := NewLocalProvider(modelConfigWithPort(portOf(t, url)), "", "", "127.0.0.1")
		if _, err := p.ProbeNativeTools(context.Background(), "m.gguf"); err == nil {
			t.Fatal("expected error for unreachable server")
		}
	})
	t.Run("no port errors", func(t *testing.T) {
		p := NewLocalProvider(models.ModelConfig{}, "", "", "127.0.0.1")
		if _, err := p.ProbeNativeTools(context.Background(), "m.gguf"); err == nil {
			t.Fatal("expected error without a port")
		}
	})
}

// portOf extracts the httptest port from a URL.
func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	p, err := strconv.Atoi(strings.TrimPrefix(rawURL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse port from %q: %v", rawURL, err)
	}
	return p
}
