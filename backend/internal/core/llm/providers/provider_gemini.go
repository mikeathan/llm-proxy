package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/models"
	"net/http"
	"strings"
)

type GeminiProvider struct {
	cfg models.ModelConfig
}

func NewGeminiProvider(cfg models.ModelConfig) *GeminiProvider {
	return &GeminiProvider{cfg: cfg}
}

func (p *GeminiProvider) Generate(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	return nil, fmt.Errorf("gemini provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *GeminiProvider) GetStatus() models.ProviderStatus {
	return models.ProviderStatusReady
}

func (p *GeminiProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	header := make(http.Header)
	key := p.cfg.ProviderConfig.APIKey
	header.Set("x-goog-api-key", key)
	header.Set("Authorization", "Bearer "+key)
	return "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", header, nil
}

func (p *GeminiProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *GeminiProvider) ListModels(ctx context.Context) ([]string, error) {
	if p.cfg.ProviderConfig.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", p.cfg.ProviderConfig.APIKey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range data.Models {
		// Names come in format "models/gemini-pro", we just want "gemini-pro"
		name := strings.TrimPrefix(m.Name, "models/")
		models = append(models, name)
	}

	return models, nil
}

func (p *GeminiProvider) TestConnection(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}

func (p *GeminiProvider) Shutdown() error {
	return nil
}
