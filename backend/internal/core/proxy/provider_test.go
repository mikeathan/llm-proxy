package proxy_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

type mockSelector struct {
	def      string
	fallback string
	err      error
}

func (m *mockSelector) SelectModels() (string, string) {
	return m.def, m.fallback
}

type dummyClient struct {
	baseURL string
}

func (d *dummyClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return &proxy.ChatResponse{}, nil
}

func (d *dummyClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return nil, errors.New("streaming not implemented in dummy")
}

func (d *dummyClient) ReasoningField() string { return proxy.ReasoningFieldBudget }

func TestRuntimeClientProvider_NoModelError(t *testing.T) {
	runtime := &mocks.MockManager{}
	selector := &mockSelector{def: ""}
	provider := proxy.NewRuntimeClientProvider(selector, runtime, nil)

	_, err := provider.GetClient(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no target model available") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRuntimeClientProvider_ModelStarting(t *testing.T) {
	selector := &mockSelector{def: "alpha"}
	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, models.ErrModelStarting
		},
	}

	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, model string, headers http.Header, local bool) proxy.Client {
		return &dummyClient{baseURL: baseURL}
	})

	if _, err := provider.GetClient(context.Background()); !errors.Is(err, models.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got %v", err)
	}
}

func TestRuntimeClientProvider_ReusesAndRebuildsClient(t *testing.T) {
	selector := &mockSelector{def: "alpha"}
	calls := 0
	var lastURL string
	activity := 0

	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			switch name {
			case "alpha":
				return llm.ModelInstance{Name: name, ModelID: "alpha-id", Host: "127.0.0.1", Port: 1234}, nil
			case "beta":
				return llm.ModelInstance{Name: name, ModelID: "beta-id", Host: "127.0.0.1", Port: 5678}, nil
			default:
				return llm.ModelInstance{}, errors.New("unexpected model")
			}
		},
		RecordActivityFunc: func(name string) {
			activity++
		},
	}

	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, model string, headers http.Header, local bool) proxy.Client {
		calls++
		lastURL = baseURL
		return &dummyClient{baseURL: baseURL}
	})

	client1, err := provider.GetClient(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 client creation, got %d", calls)
	}
	if lastURL != "http://127.0.0.1:1234" {
		t.Fatalf("unexpected base URL: %s", lastURL)
	}

	client2, err := provider.GetClient(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected client to be reused")
	}
	if client1 != client2 {
		t.Fatalf("expected same client instance to be reused")
	}

	selector.def = "beta"
	client3, err := provider.GetClient(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected client to be rebuilt, got %d", calls)
	}
	if client3 == client2 {
		t.Fatalf("expected new client instance for model change")
	}
	if activity != 3 {
		t.Fatalf("expected RecordActivity to be called per request, got %d", activity)
	}
}

// A client is built for the model's workload class, not just its URL: a local
// workload uses the llama.cpp reasoning field and the long response-header
// timeout. When the class changes (a model identified as llama.cpp after
// startup) the cached client must be rebuilt.
func TestRuntimeClientProvider_RebuildsWhenWorkloadClassChanges(t *testing.T) {
	selector := &mockSelector{def: "alpha"}
	local := false
	var builtLocal []bool

	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{Name: name, ModelID: "alpha-id", URL: "https://models.example.net/v1", Local: local}, nil
		},
	}
	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, model string, headers http.Header, isLocal bool) proxy.Client {
		builtLocal = append(builtLocal, isLocal)
		return &dummyClient{baseURL: baseURL}
	})

	for _, l := range []bool{false, false, true, true} {
		local = l
		if _, err := provider.GetClient(context.Background()); err != nil {
			t.Fatalf("GetClient: %v", err)
		}
	}
	if got := fmt.Sprint(builtLocal); got != "[false true]" {
		t.Errorf("clients built with local = %s, want [false true] (reuse until the class changes)", got)
	}
}
