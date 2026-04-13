package proxy

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/core/llm"
	"llm-proxy/models"
	"net/http"
	"sync"
)

type ModelSelector interface {
	DefaultModel() (string, error)
	PrimaryModel() string
	FallbackModel() string
}

type LLMClientProvider interface {
	GetClient(ctx context.Context) (Client, error)
	GetClientForModel(ctx context.Context, modelName string) (Client, error)
}

type RuntimeClientProvider struct {
	selector ModelSelector

	runtime llm.RuntimeManager
	mu      sync.Mutex
	client  Client
	url     string
	model   string
	headers http.Header

	newClient func(baseURL string, model string, headers http.Header) Client
}

func NewRuntimeClientProvider(
	selector ModelSelector,
	runtime llm.RuntimeManager,
	newClient func(baseURL string, model string, headers http.Header) Client) LLMClientProvider {

	return &RuntimeClientProvider{
		selector:  selector,
		runtime:   runtime,
		newClient: newClient,
	}
}

func (p *RuntimeClientProvider) GetClient(ctx context.Context) (Client, error) {
	primary := p.selector.PrimaryModel()
	fallback := p.selector.FallbackModel()

	// 1. If we have a Primary, try it first
	if primary != "" {
		client, err := p.GetClientForModel(ctx, primary)
		if err == nil {
			return client, nil
		}

		// Strictly honor the "Starting" state - do not fallback if it's just a cold start
		if errors.Is(err, models.ErrModelStarting) {
			return nil, err
		}

		// For any other "Terminal" error, try to use the fallback if available
		if fallback != "" {
			return p.GetClientForModel(ctx, fallback)
		}

		return nil, err
	}

	// 2. Legacy/Fallback: Use DefaultModel if no Primary is configured
	modelName, err := p.selector.DefaultModel()
	if err != nil {
		return nil, err
	}
	return p.GetClientForModel(ctx, modelName)
}

func (p *RuntimeClientProvider) GetClientForModel(ctx context.Context, modelName string) (Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	inst, err := p.runtime.GetInstance(ctx, modelName)
	if err != nil {
		return nil, err
	}

	p.ensureClient(inst, modelName)
	p.runtime.RecordActivity(modelName)

	return p.client, nil
}

func (p *RuntimeClientProvider) ensureClient(inst llm.ModelInstance, modelName string) {
	baseURL := inst.URL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", inst.Host, inst.Port)
	}

	// Rebuild if:
	// - no client yet
	// - requested model differs from cached model
	// - URL changed (port/host changed)
	// - headers changed
	if p.client == nil || p.model != modelName || p.url != baseURL || !compareHeaders(p.headers, inst.Headers) {
		p.client = p.newClient(baseURL, inst.ModelID, inst.Headers)
		p.model = modelName
		p.url = baseURL
		p.headers = inst.Headers
	}
}

func compareHeaders(h1, h2 http.Header) bool {
	if len(h1) != len(h2) {
		return false
	}
	for k, v1 := range h1 {
		v2, ok := h2[k]
		if !ok || len(v1) != len(v2) {
			return false
		}
		for i := range v1 {
			if v1[i] != v2[i] {
				return false
			}
		}
	}
	return true
}
