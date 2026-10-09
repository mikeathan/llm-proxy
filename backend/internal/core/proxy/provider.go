package proxy

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/network"
	"llm-proxy/models"
	"net/http"
	"sync"
	"time"
)

type ModelSelector interface {
	SelectModels() (primary string, fallback string)
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
	local   bool

	newClient func(baseURL string, model string, headers http.Header, local bool) Client
}

func NewRuntimeClientProvider(
	selector ModelSelector,
	runtime llm.RuntimeManager,
	newClient func(baseURL string, model string, headers http.Header, local bool) Client) LLMClientProvider {

	return &RuntimeClientProvider{
		selector:  selector,
		runtime:   runtime,
		newClient: newClient,
	}
}

func (p *RuntimeClientProvider) GetClient(ctx context.Context) (Client, error) {
	primary, fallback := p.selector.SelectModels()
	if primary == "" {
		return nil, fmt.Errorf("no target model available")
	}

	client, err := p.GetClientForModel(ctx, primary)
	if err == nil {
		return client, nil
	}

	// Failover: if not cold starting and fallback exists, try it
	if !errors.Is(err, models.ErrModelStarting) && fallback != "" && fallback != primary {
		return p.GetClientForModel(ctx, fallback)
	}

	return nil, err
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
		baseURL = network.FormatURL(inst.Host, inst.Port)
	}

	// Rebuild if:
	// - no client yet
	// - requested model differs from cached model
	// - URL changed (port/host changed)
	// - headers changed
	// - workload class changed (the client's transport and reasoning field follow it)
	if p.client == nil || p.model != modelName || p.url != baseURL || !compareHeaders(p.headers, inst.Headers) || p.local != inst.Local {
		p.client = p.newClient(baseURL, inst.ModelID, inst.Headers, inst.Local)
		p.model = modelName
		p.url = baseURL
		p.headers = inst.Headers
		p.local = inst.Local
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

// ModelWait bounds a wait for a model that is still loading.
type ModelWait struct {
	ModelName    string
	PollInterval time.Duration
	Timeout      time.Duration
	// OnStarting, if set, is called once, the first time the model reports it is still starting — so a caller
	// can tell the user the model is loading instead of looking stuck.
	OnStarting func()
}

// WaitForClient calls get until it returns a client. models.ErrModelStarting means "still loading" and is polled
// every PollInterval; any other error is returned at once. It gives up after Timeout or when ctx is done. A cold
// local model is started by the first get, so the first request after an idle unload or a restart waits for it
// instead of failing.
func WaitForClient(ctx context.Context, get func() (Client, error), w ModelWait) (Client, error) {
	waitCtx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	announced := false
	for {
		client, err := get()
		if err == nil {
			return client, nil
		}
		if !errors.Is(err, models.ErrModelStarting) {
			return nil, err
		}
		if !announced && w.OnStarting != nil {
			w.OnStarting()
			announced = true
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("model %s did not become ready within %v: %w", w.ModelName, w.Timeout, err)
		case <-time.After(w.PollInterval):
		}
	}
}
