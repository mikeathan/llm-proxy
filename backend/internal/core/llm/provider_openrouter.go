package llm

import (
	"context"
	"encoding/json"
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

func (p *OpenRouterProvider) ListModels(ctx context.Context) ([]string, error) {
	url := "https://openrouter.ai/api/v1/models"
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
		return nil, fmt.Errorf("openrouter API returned status %d", resp.StatusCode)
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

func (p *OpenRouterProvider) TestConnection(ctx context.Context) error {
	if p.cfg.ProviderConfig.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	url := "https://openrouter.ai/api/v1/auth/key"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.ProviderConfig.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("invalid API key")
		}
		return fmt.Errorf("openrouter API returned status %d", resp.StatusCode)
	}

	return nil
}

func (p *OpenRouterProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *OpenRouterProvider) Shutdown() error {
	return nil
}
