package llm

import (
	"context"
	"fmt"
	"llm-proxy/models"
	"net/http"
)

type GeminiProvider struct {
	cfg models.ModelConfig
}

func NewGeminiProvider(cfg models.ModelConfig) *GeminiProvider {
	return &GeminiProvider{cfg: cfg}
}

func (p *GeminiProvider) Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("gemini provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *GeminiProvider) GetStatus() ProviderStatus {
	return ProviderStatusReady
}

func (p *GeminiProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	header := make(http.Header)
	header.Set("x-goog-api-key", p.cfg.ProviderConfig.APIKey)
	return "https://generativelanguage.googleapis.com/v1beta/openai", header, nil
}

func (p *GeminiProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *GeminiProvider) Shutdown() error {
	return nil
}
