// OpenAICompatibleProvider handles any OpenAI-compatible API endpoint,
// including OpenRouter and NVIDIA.  The ListModels method parses the
// optional pricing block from the response so ICU weights can be
// auto-computed at model registration time — no static cost tables.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAICompatibleProvider handles providers that follow the OpenAI API schema.
type OpenAICompatibleProvider struct {
	cfg      models.ModelConfig
	manifest models.ProviderManifest
	doer     HTTPDoer
	// workloadClassifier is injected by the registrar (model host + local
	// interface IPs) for the §3.4 /slots gate and local probing.
	workloadClassifier models.WorkloadClassifier

	// cache is the catalog listing cache.  The registrar injects a SHARED
	// *core.TTLCache (owned by the registrar, keyed by effective models URL) so
	// OpenRouter's full catalog is read once per TTL across all provider
	// instances — never hardcoded, never refetched per discovery request
	// (§2.10 #3).  Direct construction (tests) gets a private cache so a single
	// instance still caches.
	cache *core.TTLCache[string, []models.ProviderModelInfo]
}

// catalogTTL bounds how long a provider catalog listing stays valid.
const catalogTTL = time.Hour

// catalogCacheMaxEntries bounds the shared catalog cache so a pathological
// number of distinct endpoints cannot grow it without bound.
const catalogCacheMaxEntries = 64

func NewOpenAICompatibleProvider(cfg models.ModelConfig, manifest models.ProviderManifest) *OpenAICompatibleProvider {
	return NewOpenAICompatibleProviderWithDoer(cfg, manifest, nil)
}

// NewOpenAICompatibleProviderWithDoer injects the dedicated provider HTTP
// client.  A nil doer falls back to the shared pooled transport (bootstrap
// injects it through the factory).
func NewOpenAICompatibleProviderWithDoer(cfg models.ModelConfig, manifest models.ProviderManifest, doer HTTPDoer) *OpenAICompatibleProvider {
	if doer == nil {
		doer = defaultProviderDoer()
	}
	return &OpenAICompatibleProvider{
		cfg:      cfg,
		manifest: manifest,
		doer:     doer,
		cache:    core.NewTTLCache[string, []models.ProviderModelInfo](catalogCacheMaxEntries, catalogTTL, nil),
	}
}

// SetHTTPDoer swaps the injected provider HTTP client (called by the registrar
// at build time).
func (p *OpenAICompatibleProvider) SetHTTPDoer(doer HTTPDoer) {
	p.doer = doer
}

// SetCatalogCache replaces the per-instance cache with the registrar's shared
// catalog cache so listings are reused across provider builds (called by the
// registrar at build time).
func (p *OpenAICompatibleProvider) SetCatalogCache(cache *core.TTLCache[string, []models.ProviderModelInfo]) {
	if cache != nil {
		p.cache = cache
	}
}

// SetWorkloadClassifier injects the shared workload classifier (model host +
// local interface IPs) used for the §3.4 /slots gate.
func (p *OpenAICompatibleProvider) SetWorkloadClassifier(c models.WorkloadClassifier) {
	p.workloadClassifier = c
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

	endpoint := p.manifest.Endpoints.Chat
	if endpoint == "" {
		endpoint = "/chat/completions"
	}

	return p.endpointURL(endpoint), header, nil
}

func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) ([]models.ProviderModelInfo, error) {
	// Serve from the shared TTL cache while fresh (V5).  The cache is keyed by
	// the effective models endpoint so different providers/overrides never
	// collide, and the registrar's shared cache makes the catalog a read-once
	// per TTL across every discovery call.  Get is single-flight: concurrent
	// misses share one catalog fetch.
	return p.cache.Get(p.modelsEndpoint(), func() ([]models.ProviderModelInfo, error) {
		return p.fetchModels(ctx)
	})
}

// endpointURL joins the effective base URL and an endpoint path, normalizing
// the trailing slash — the single place this join happens.
func (p *OpenAICompatibleProvider) endpointURL(endpoint string) string {
	return strings.TrimSuffix(p.effectiveBaseURL(), "/") + endpoint
}

// modelsEndpoint returns the effective models-list URL, used both as the fetch
// target and as the catalog cache key.
func (p *OpenAICompatibleProvider) modelsEndpoint() string {
	endpoint := p.manifest.Endpoints.Models
	if endpoint == "" {
		endpoint = "/models"
	}
	return p.endpointURL(endpoint)
}

// fetchModels performs the network catalog fetch (uncached).
func (p *OpenAICompatibleProvider) fetchModels(ctx context.Context) ([]models.ProviderModelInfo, error) {
	url := p.modelsEndpoint()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	p.setAuthHeaders(req.Header)

	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s API (%s) returned status %d", p.manifest.Name, url, resp.StatusCode)
	}

	var data struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	// Parse each model's capabilities via the alias-list walker (§2.10 #2):
	// context_length / top_provider.{context_length,max_completion_tokens} /
	// pricing — one parser across every provider's JSON shape.
	var out []models.ProviderModelInfo
	for _, raw := range data.Data {
		var node map[string]any
		if err := json.Unmarshal(raw, &node); err != nil {
			continue
		}
		info := models.ProviderModelInfo{ID: stringID(node["id"])}
		if cap := extractCapability(node, OutputCapKeys); cap > 0 {
			info.MaxOutputTokens = cap
		}
		if ctxLen := extractCapability(node, ContextLengthKeys); ctxLen > 0 {
			info.ContextLength = ctxLen
		}
		if pricing := parsePricing(node); pricing != nil {
			info.Pricing = pricing
		}
		out = append(out, info)
	}

	// /slots probe (llama.cpp serving context n_ctx) runs ONLY for effective
	// local endpoints (§3.4) — never for cloud URLs, preserving the wasted-calls
	// fix.  For non-.gguf local models (M8), the probe — not providerCtxDefaults
	// — recovers the real serving n_ctx.
	if p.isEffectiveLocal(p.effectiveBaseURL()) {
		if slotCtx := p.fetchSlotsContext(ctx, p.effectiveBaseURL()); slotCtx > 0 {
			for i := range out {
				if out[i].ContextLength == 0 {
					out[i].ContextLength = slotCtx
				}
			}
		}
	}

	return out, nil
}

// effectiveBaseURL returns the provider's base URL (or manifest default).
func (p *OpenAICompatibleProvider) effectiveBaseURL() string {
	if p.cfg.ProviderConfig.BaseURL != "" {
		return p.cfg.ProviderConfig.BaseURL
	}
	return p.manifest.DefaultBaseURL
}

// isEffectiveLocal reports whether the resolved base URL targets a local
// serving host, gating the /slots probe (§3.4).  The injected classifier
// (model host + local interface IPs) decides.  When no classifier was injected
// (direct construction outside the registrar), we conservatively do NOT probe —
// cloud listings never see futile /slots calls, and local metadata discovery is
// restored by the registrar-injected classifier on the runtime path.
func (p *OpenAICompatibleProvider) isEffectiveLocal(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	if !p.workloadClassifier.HasContext() {
		return false
	}
	return p.workloadClassifier.ClassifyEndpoint(baseURL)
}

// stringID extracts a string field from a decoded model node.
func stringID(v any) string {
	s, _ := v.(string)
	return s
}

// parsePricing extracts an optional pricing block from a model node.
func parsePricing(node map[string]any) *models.ModelPricing {
	p, ok := node["pricing"].(map[string]any)
	if !ok {
		return nil
	}
	prompt, _ := p["prompt"].(string)
	completion, _ := p["completion"].(string)
	if prompt == "" && completion == "" {
		return nil
	}
	return &models.ModelPricing{Prompt: prompt, Completion: completion}
}

// fetchSlotsContext queries GET /slots on a llama.cpp server and returns
// n_ctx from the first idle slot, or 0 if the endpoint is unavailable.
// The baseURL may include a path prefix (e.g. /v1) that MUST be stripped
// since /slots lives at the server root, not under /v1.  Runs only for
// effective local endpoints (§3.4) with a 5s child context (S2).
func (p *OpenAICompatibleProvider) fetchSlotsContext(ctx context.Context, baseURL string) int {
	ctx, cancel := context.WithTimeout(ctx, slotsProbeTimeout)
	defer cancel()

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

	resp, err := p.doer.Do(req)
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
	if value == "" && p.cfg.ProviderConfig.APIKeyName != "" {
		// Defense-in-depth: a request that explicitly names a credential but
		// reaches the wire with an empty key is always a misconfiguration
		// (Build should have caught it).  Fail loudly so a 401 is not mistaken
		// for an upstream problem.  An unkeyed, unnamed endpoint (no-auth) is
		// left silent.
		logging.Error("provider request sent without its named API key", "provider", p.cfg.Provider, "credential", p.cfg.ProviderConfig.APIKeyName)
	}
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
