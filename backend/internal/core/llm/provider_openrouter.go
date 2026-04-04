package llm

import (
	"context"
	"fmt"
	"llm-proxy/models"
	"net/http"
)

type OpenRouterProvider struct {
	cfg models.ModelConfig
}

func NewOpenRouterProvider(cfg models.ModelConfig) *OpenRouterProvider {
	return &OpenRouterProvider{cfg: cfg}
}

func (p *OpenRouterProvider) Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("openrouter provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *OpenRouterProvider) GetStatus() ProviderStatus {
	return ProviderStatusReady
}

func (p *OpenRouterProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+p.cfg.ProviderConfig.APIKey)
	return "https://openrouter.ai/api", header, nil
}

func (p *OpenRouterProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *OpenRouterProvider) Shutdown() error {
	return nil
}
