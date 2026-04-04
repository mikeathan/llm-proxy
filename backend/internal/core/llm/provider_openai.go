package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/models"
	"net/http"
	"strings"
)

type OpenAIProvider struct {
	cfg models.ModelConfig
}

func NewOpenAIProvider(cfg models.ModelConfig) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg}
}

func (p *OpenAIProvider) Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("openai provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *OpenAIProvider) GetStatus() ProviderStatus {
	return ProviderStatusReady
}

func (p *OpenAIProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+p.cfg.ProviderConfig.APIKey)
	baseURL := p.cfg.ProviderConfig.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return baseURL, header, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	baseURL := p.cfg.ProviderConfig.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	
	url := strings.TrimSuffix(baseURL, "/") + "/models"
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.ProviderConfig.APIKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API returned status %d", resp.StatusCode)
	}

	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range data.Data {
		models = append(models, m.ID)
	}

	return models, nil
}

func (p *OpenAIProvider) TestConnection(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}

func (p *OpenAIProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *OpenAIProvider) Shutdown() error {
	return nil
}
