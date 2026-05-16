// OpenAICompatibleProvider handles any OpenAI-compatible API endpoint,
// including OpenRouter and NVIDIA.  The ListModels method parses the
// optional pricing block from the response so ICU weights can be
// auto-computed at model registration time — no static cost tables.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/models"
	"net/http"
	"strings"
)

// OpenAICompatibleProvider handles providers that follow the OpenAI API schema.
type OpenAICompatibleProvider struct {
	cfg      models.ModelConfig
	manifest models.ProviderManifest
}

func NewOpenAICompatibleProvider(cfg models.ModelConfig, manifest models.ProviderManifest) *OpenAICompatibleProvider {
	return &OpenAICompatibleProvider{
		cfg:      cfg,
		manifest: manifest,
	}
}

func (p *OpenAICompatibleProvider) Generate(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	return nil, fmt.Errorf("%s provider Chat endpoint is not yet implemented natively; use standard model host proxying", p.manifest.Name)
}

func (p *OpenAICompatibleProvider) GetStatus() models.ProviderStatus {
	return models.ProviderStatusReady
}

func (p *OpenAICompatibleProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	header := make(http.Header)
	p.setAuthHeaders(header)

	baseURL := p.cfg.ProviderConfig.BaseURL
	if baseURL == "" {
		baseURL = p.manifest.DefaultBaseURL
	}

	endpoint := p.manifest.Endpoints.Chat
	if endpoint == "" {
		endpoint = "/chat/completions"
	}

	url := strings.TrimSuffix(baseURL, "/") + endpoint
	return url, header, nil
}

func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) ([]models.ProviderModelInfo, error) {
	baseURL := p.cfg.ProviderConfig.BaseURL
	if baseURL == "" {
		baseURL = p.manifest.DefaultBaseURL
	}

	endpoint := p.manifest.Endpoints.Models
	if endpoint == "" {
		endpoint = "/models"
	}

	url := strings.TrimSuffix(baseURL, "/") + endpoint

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	p.setAuthHeaders(req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s API (%s) returned status %d", p.manifest.Name, url, resp.StatusCode)
	}

	var data struct {
		Data []struct {
			ID      string              `json:"id"`
			Pricing *models.ModelPricing `json:"pricing,omitempty"`
			Limits  *models.ModelLimits  `json:"limits,omitempty"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var out []models.ProviderModelInfo
	for _, m := range data.Data {
		out = append(out, models.ProviderModelInfo{
			ID:      m.ID,
			Pricing: m.Pricing,
			Limits:  m.Limits,
		})
	}

	return out, nil
}

func (p *OpenAICompatibleProvider) setAuthHeaders(header http.Header) {
	headerName := p.manifest.Auth.HeaderName
	if headerName == "" {
		headerName = "Authorization"
	}

	value := p.cfg.ProviderConfig.APIKey
	if p.manifest.Auth.HeaderPrefix != "" {
		value = p.manifest.Auth.HeaderPrefix + " " + value
	} else if p.manifest.Auth.Type == "bearer" && !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = "Bearer " + value
	}

	header.Set(headerName, value)
}

func (p *OpenAICompatibleProvider) TestConnection(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}

func (p *OpenAICompatibleProvider) EnsureReady(ctx context.Context) error {
	return nil
}

func (p *OpenAICompatibleProvider) Shutdown() error {
	return nil
}
