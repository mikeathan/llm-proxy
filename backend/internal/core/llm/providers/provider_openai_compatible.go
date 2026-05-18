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
	"net/url"
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
			Meta    *models.ModelMeta    `json:"meta,omitempty"`
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
			Meta:    m.Meta,
		})
	}

	// Query /slots to get the actual serving context (n_ctx).
	// llama.cpp reports n_ctx_train in /v1/models (training context,
	// often 262K) but the real serving context is n_ctx (e.g. 8K).
	// This is the same approach production frameworks use: probe all
	// available endpoints to resolve the true context window.
	if slotCtx := p.fetchSlotsContext(ctx, baseURL); slotCtx > 0 {
		for i := range out {
			if out[i].Meta == nil {
				out[i].Meta = &models.ModelMeta{Nctx: slotCtx}
			} else if out[i].Meta.Nctx == 0 {
				out[i].Meta.Nctx = slotCtx
			}
		}
	}

	return out, nil
}

// fetchSlotsContext queries GET /slots on a llama.cpp server and returns
// n_ctx from the first idle slot, or 0 if the endpoint is unavailable.
// The baseURL may include a path prefix (e.g. /v1) that MUST be stripped
// since /slots lives at the server root, not under /v1.
func (p *OpenAICompatibleProvider) fetchSlotsContext(ctx context.Context, baseURL string) int {
	// Try /slots first (llama.cpp root endpoint)
	if n := p.trySlotsURL(ctx, baseURL, "/slots"); n > 0 {
		return n
	}
	// Fallback: try /v1/slots (some servers expose it there)
	return p.trySlotsURL(ctx, baseURL, "/v1/slots")
}

func (p *OpenAICompatibleProvider) trySlotsURL(ctx context.Context, baseURL, path string) int {
	u, err := url.Parse(baseURL)
	if err != nil {
		return 0
	}
	u.RawPath = ""
	u.Path = path
	slotsURL := u.String()
	req, err := http.NewRequestWithContext(ctx, "GET", slotsURL, nil)
	if err != nil {
		return 0
	}
	p.setAuthHeaders(req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var slots []struct {
		Nctx int `json:"n_ctx"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&slots); err != nil {
		return 0
	}
	for _, s := range slots {
		if s.Nctx > 0 {
			return s.Nctx
		}
	}
	return 0
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
