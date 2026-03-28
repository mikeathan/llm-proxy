package proxy

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/llm"
	"sync"
)

type ModelSelector interface {
	DefaultModel() (string, error)
}

type LLMClientProvider interface {
	GetClient(ctx context.Context) (Client, error)
}

type RuntimeClientProvider struct {
	selector ModelSelector

	runtime llm.RuntimeManager
	mu      sync.Mutex
	client  Client
	url     string
	model   string

	newClient func(baseURL string) Client
}

func NewRuntimeClientProvider(
	selector ModelSelector,
	runtime llm.RuntimeManager,
	newClient func(baseURL string) Client) LLMClientProvider {

	return &RuntimeClientProvider{
		selector:  selector,
		runtime:   runtime,
		newClient: newClient,
	}
}

func (p *RuntimeClientProvider) GetClient(ctx context.Context) (Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	modelName, err := p.selector.DefaultModel()
	if err != nil {
		return nil, err
	}

	inst, err := p.runtime.EnsureModel(ctx, modelName)
	if err != nil {
		if errors.Is(err, llm.ErrModelStarting) {
			return nil, err
		}
		return nil, err
	}

	p.ensureClient(inst, modelName)

	p.runtime.RecordActivity(modelName)

	return p.client, nil
}

func (p *RuntimeClientProvider) ensureClient(inst llm.ModelInstance, modelName string) {

	// Rebuild if:
	// - no client yet
	// - requested model differs from cached model
	// - URL changed (port/host changed)
	baseURL := fmt.Sprintf("http://%s:%d", inst.Host, inst.Port)

	if p.client == nil || p.model != modelName || p.url != baseURL {
		p.client = p.newClient(baseURL)
		p.model = modelName
		p.url = baseURL
	}
}
