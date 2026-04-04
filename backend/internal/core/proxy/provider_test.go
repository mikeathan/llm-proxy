package proxy_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/testing/mocks"
)

type mockSelector struct {
	model string
	err   error
}

func (m *mockSelector) DefaultModel() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.model, nil
}

type dummyClient struct {
	baseURL string
}

func (d *dummyClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return &proxy.ChatResponse{}, nil
}

func TestRuntimeClientProvider_DefaultModelError(t *testing.T) {
	selector := &mockSelector{err: errors.New("no default")}
	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			t.Fatalf("EnsureModel should not be called")
			return llm.ModelInstance{}, nil
		},
	}

	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, headers http.Header) proxy.Client {
		return &dummyClient{baseURL: baseURL}
	})

	if _, err := provider.GetClient(context.Background()); err == nil {
		t.Fatalf("expected error when default model fails")
	}
}

func TestRuntimeClientProvider_ModelStarting(t *testing.T) {
	selector := &mockSelector{model: "alpha"}
	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, llm.ErrModelStarting
		},
	}

	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, headers http.Header) proxy.Client {
		return &dummyClient{baseURL: baseURL}
	})

	if _, err := provider.GetClient(context.Background()); !errors.Is(err, llm.ErrModelStarting) {
		t.Fatalf("expected ErrModelStarting, got %v", err)
	}
}

func TestRuntimeClientProvider_ReusesAndRebuildsClient(t *testing.T) {
	selector := &mockSelector{model: "alpha"}
	calls := 0
	var lastURL string
	activity := 0

	manager := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			switch name {
			case "alpha":
				return llm.ModelInstance{Name: name, Host: "127.0.0.1", Port: 1234}, nil
			case "beta":
				return llm.ModelInstance{Name: name, Host: "127.0.0.1", Port: 5678}, nil
			default:
				return llm.ModelInstance{}, errors.New("unexpected model")
			}
		},
		RecordActivityFunc: func(name string) {
			activity++
		},
	}

	provider := proxy.NewRuntimeClientProvider(selector, manager, func(baseURL string, headers http.Header) proxy.Client {
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

	selector.model = "beta"
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
