// OpenAICompatibleProvider handles any OpenAI-compatible API endpoint,
// including OpenRouter and NVIDIA.  The ListModels method parses the
// optional pricing block from the response so ICU weights can be
// auto-computed at model registration time — no static cost tables.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// probeChatTimeout bounds the model-availability probe so an unresponsive
// upstream (e.g. a model the key is not entitled to whose gateway hangs or
// resets the connection instead of returning a clean error) cannot stall the
// registration request indefinitely.
const probeChatTimeout = 10 * time.Second

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

	// /slots probe (llama.cpp serving context n_ctx) recovers the REAL serving
	// window for local workloads.  It runs for effective-local endpoints (§3.4)
	// AND for any listing that serves local llama.cpp workloads — a REMOTE
	// llama.cpp host (not loopback/modelHost) is still a local workload when the
	// listing itself carries the llama.cpp fingerprint (owned_by "llamacpp" /
	// meta.n_ctx_train) or a .gguf artifact id.  Cloud catalogs never match,
	// preserving the wasted-calls fix.  The probe result OVERRIDES the
	// training-derived ContextLength (n_ctx_train): /slots n_ctx is the actual
	// server window the local budget must key on (SPEC-005 priority 1), and it
	// is carried on Meta.Nctx so discovery forwards the serving context.
	if p.isEffectiveLocal(p.effectiveBaseURL()) || listingServesLocalWorkload(data.Data) {
		if slotCtx := p.fetchSlotsContext(ctx, p.effectiveBaseURL()); slotCtx > 0 {
			for i := range out {
				out[i].ContextLength = slotCtx
				if out[i].Meta == nil {
					out[i].Meta = &models.ModelMeta{}
				}
				out[i].Meta.Nctx = slotCtx
			}
		}
	}

	return out, nil
}

// listingServesLocalWorkload reports whether any model in a /v1/models listing
// identifies a llama.cpp server serving local GGUF models: owned_by
// "llamacpp", a meta.n_ctx_train field, or a .gguf artifact id.  Data-driven
// and host-agnostic — a remote llama.cpp host must still be probed for its
// serving n_ctx, while cloud catalogs (OpenRouter/NVIDIA/OpenAI) never match
// and keep the §3.4 wasted-calls fix.
//
// This gate is deliberately GENERIC: other local server types (LM Studio,
// vLLM, …) already match via n_ctx_train / .gguf without code changes.  Only
// the probe itself is llama.cpp-specific (/slots); a server without that
// endpoint makes fetchSlotsContext return 0 and nothing changes.  Adding a
// new server type later = add its probe branch here (e.g. LM Studio
// /api/v1/models, vLLM /version — §2.10 #4), never touch this gate.
func listingServesLocalWorkload(nodes []json.RawMessage) bool {
	for _, raw := range nodes {
		var node map[string]any
		if err := json.Unmarshal(raw, &node); err != nil {
			continue
		}
		if owned, _ := node["owned_by"].(string); strings.EqualFold(owned, "llamacpp") {
			return true
		}
		if meta, ok := node["meta"].(map[string]any); ok {
			if v, ok := meta["n_ctx_train"]; ok && coerceInt(v) > 0 {
				return true
			}
		}
		if models.HasGGUFArtifact(stringID(node["id"])) {
			return true
		}
	}
	return false
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

// ProbeChatModel issues a minimal chat request (max_tokens=1) to verify the
// given model ID actually resolves on the provider endpoint. Public catalogs
// list models a key is not entitled to, so a real chat probe is the only
// reliable way to distinguish "available" from "listed but not callable"
// (NVIDIA, for example, returns 404 or resets the connection for unprovisioned
// models). The probe is bounded by probeChatTimeout.
func (p *OpenAICompatibleProvider) ProbeChatModel(ctx context.Context, modelID string) error {
	ctx, cancel := context.WithTimeout(ctx, probeChatTimeout)
	defer cancel()

	endpoint, headers, err := p.GetEndpoint(ctx)
	if err != nil {
		return fmt.Errorf("resolve endpoint: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		return fmt.Errorf("marshal probe request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := p.doer.Do(req)
	if err != nil {
		return fmt.Errorf("model %q unreachable: %w", modelID, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = "(no response body)"
		}
		return fmt.Errorf("model %q not callable: upstream status %d: %s", modelID, resp.StatusCode, msg)
	}
	return nil
}

// DefaultProbeNativeToolTimeout bounds a single native-tool probe attempt.
// It rides on the same inference slot as real traffic (llama.cpp serves one
// slot), so a hung probe would block the agent's first turn. Timeouts are
// GENERATION-TIME based (not token based) so slow hardware gets the same
// chance as fast hardware: the deadline bounds how long the server may take
// to emit a tool call, and the budget bounds how many tokens it may spend
// thinking first.
const DefaultProbeNativeToolTimeout = 30 * time.Second

// DefaultProbeNativeToolMaxTimeout is the deadline for the final escalation
// attempt, reserved for models that were still generating (length-limited or
// past the first deadline) — generous enough for slow large models on CPU.
const DefaultProbeNativeToolMaxTimeout = 60 * time.Second

// ProbeNativeToolTimeout allows overriding the per-attempt probe deadline for
// testing.
var ProbeNativeToolTimeout = DefaultProbeNativeToolTimeout

// ProbeNativeToolMaxTimeout allows overriding the escalation deadline for
// testing.
var ProbeNativeToolMaxTimeout = DefaultProbeNativeToolMaxTimeout

// probeNativeToolBudget is the first-attempt output budget: room for a
// thinking model to emit its reasoning AND the tool call (~40 thinking + ~30
// call tokens on the reference model).
const probeNativeToolBudget = 512

// probeNativeToolMaxBudget is the final-attempt budget: covers models that
// think substantially before acting before we conclude "no native support".
const probeNativeToolMaxBudget = 2048

// ProbeNativeTools asks the endpoint whether it can emit native OpenAI tool
// calls for the given model. The probe sends a minimal chat request carrying a
// trivial function schema and inspects the response: finish_reason
// "tool_calls" (or a non-empty message.tool_calls) means the server rendered
// the model's chat template with tools; a plain-text response means it cannot.
//
// The probe must stay correct across every model class that can hit it:
//
//   - THINKING models spend tokens on reasoning_content before acting; a
//     budget too small ends with finish_reason "length" and zero tool calls —
//     a false "no native support" that silently drops the model into XML text
//     mode. The budget ladder (512 → 2048) covers verbose thinkers.
//   - SLOW hardware (big quantized models on CPU) generates few tokens per
//     second; timeouts are generation-time based (30s → 60s), not token
//     based, so a slow model gets the same wall-clock chance as a fast one.
//     A deadline-exceeded first attempt escalates once instead of failing.
//   - DEAD or refusing servers fail immediately (connection errors are not
//     escalated) so a down endpoint never stalls the agent for the full
//     ladder.
//
// This is the capability signal that lets local OpenAI-compatible models
// (llama.cpp --jinja, Ollama, LM Studio, vLLM, ...) use native function calling
// without a per-model tool_call_format setting. Returns (false, nil) for a
// healthy endpoint without native tool support; (false, err) for transport or
// upstream errors so callers can fall back to XML text mode without caching.
func (p *OpenAICompatibleProvider) ProbeNativeTools(ctx context.Context, modelID string) (bool, error) {
	endpoint, headers, err := p.GetEndpoint(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve endpoint: %w", err)
	}

	// Attempt 1: reasonable budget and deadline — the fast path.
	supported, lengthLimited, err := p.probeNativeToolsOnce(ctx, endpoint, headers, modelID, probeNativeToolBudget, ProbeNativeToolTimeout)
	if err != nil {
		// A deadline-exceeded first attempt means the server was alive but
		// slow (still generating when we gave up) — escalate once with the
		// generous budget/deadline. Any other transport error (connection
		// refused, reset, non-200) is terminal — fail fast.
		if errors.Is(err, context.DeadlineExceeded) {
			supported, _, err = p.probeNativeToolsOnce(ctx, endpoint, headers, modelID, probeNativeToolMaxBudget, ProbeNativeToolMaxTimeout)
			return supported, err
		}
		return false, err
	}
	if supported || !lengthLimited {
		return supported, nil
	}
	// Attempt 1 was truncated by the token budget before a tool call — the
	// model may think longer before acting. Retry with the generous budget
	// under the longer deadline.
	supported, _, err = p.probeNativeToolsOnce(ctx, endpoint, headers, modelID, probeNativeToolMaxBudget, ProbeNativeToolMaxTimeout)
	return supported, err
}

// probeNativeToolsOnce performs a single native-tool probe request. Returns
// lengthLimited=true when the response was cut off by the token budget with no
// tool call (finish_reason "length"), so the caller can decide whether to
// retry with a larger budget.
func (p *OpenAICompatibleProvider) probeNativeToolsOnce(ctx context.Context, endpoint string, headers http.Header, modelID string, maxTokens int, timeout time.Duration) (supported bool, lengthLimited bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type probeTool struct {
		Type     string `json:"type"`
		Function struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  struct {
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
			} `json:"parameters"`
		} `json:"function"`
	}
	var tool probeTool
	tool.Type = "function"
	tool.Function.Name = "list_directory"
	tool.Function.Description = "List files in a directory"
	tool.Function.Parameters.Type = "object"
	tool.Function.Parameters.Properties = map[string]any{
		"path": map[string]string{"type": "string"},
	}

	body, err := json.Marshal(map[string]any{
		"model":       modelID,
		"messages":    []map[string]string{{"role": "user", "content": "List the current directory using the available tool."}},
		"tools":       []any{tool},
		"tool_choice": "auto",
		"max_tokens":  maxTokens,
	})
	if err != nil {
		return false, false, fmt.Errorf("marshal probe request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, false, fmt.Errorf("build probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := p.doer.Do(req)
	if err != nil {
		return false, false, fmt.Errorf("native-tools probe unreachable: %w", err)
	}
	defer resp.Body.Close()
	b, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		// e.g. the deadline fired mid-body on a slow server — escalate like a
		// request deadline so slow hardware still gets its chance.
		return false, false, fmt.Errorf("read probe response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = "(no response body)"
		}
		return false, false, fmt.Errorf("native-tools probe: upstream status %d: %s", resp.StatusCode, msg)
	}

	var parsed struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return false, false, nil // unrecognizable body — treat as no native support
	}
	if len(parsed.Choices) == 0 {
		return false, false, nil
	}
	ch := parsed.Choices[0]
	if ch.FinishReason == "tool_calls" || len(ch.Message.ToolCalls) > 0 {
		return true, false, nil
	}
	if ch.FinishReason == "length" {
		// Token budget exhausted before a tool call (thinking models spend
		// tokens on reasoning first) — inconclusive, retry with more budget.
		return false, true, nil
	}
	return false, false, nil
}

func (p *OpenAICompatibleProvider) EnsureReady(ctx context.Context) error {
	return nil
}
