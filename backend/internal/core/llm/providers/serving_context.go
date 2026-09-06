// serving_context.go — ServingContext: what serving context window does the
// running server report? The runtime reconciles a local model's stored
// metadata against this value at instance-ready time, because the running
// server's window (not GGUF metadata, not launch-arg strings) is the
// authoritative input the local budget must key on (SPEC-005 / §3.4).
package providers

import (
	"context"
	"encoding/json"
	"net/http"

	"llm-proxy/internal/platform/network"
)

// ServingContext reports the serving context window the running server
// actually serves: /slots n_ctx (llama.cpp root endpoint, authoritative),
// falling back to the /v1/models listing's n_ctx. 0 when unreachable or
// nothing reports a window. Bounded by slotsProbeTimeout.
func (p *OpenAICompatibleProvider) ServingContext(ctx context.Context) int {
	if n := p.fetchSlotsContext(ctx, p.effectiveBaseURL()); n > 0 {
		return n
	}
	return modelsServingContext(ctx, p.modelsEndpoint(), p.doer)
}

// ServingContext reports the serving context window of the locally-launched
// llama-server (same probe as the OpenAI-compatible path). 0 when the model is
// not configured with a port or the server is unreachable.
func (p *LocalProvider) ServingContext(ctx context.Context) int {
	if p.cfg.Port <= 0 {
		return 0
	}
	client := &http.Client{Transport: network.LLMChatTransport}
	base := network.FormatLocalURL(p.host, p.cfg.Port)
	if n := fetchSlotsNctx(ctx, base+"/slots", client); n > 0 {
		return n
	}
	return modelsServingContext(ctx, base+"/v1/models", client)
}

// modelsServingContext fetches an OpenAI-compatible /models listing and
// returns the first model's reported context window (n_ctx preferred over
// n_ctx_train by ContextLengthKeys). 0 when unreachable, non-200, or unparsed.
// Bounded by slotsProbeTimeout.
func modelsServingContext(ctx context.Context, modelsURL string, doer HTTPDoer) int {
	ctx, cancel := context.WithTimeout(ctx, slotsProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return 0
	}
	resp, err := doer.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var data struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0
	}
	for _, raw := range data.Data {
		var node map[string]any
		if json.Unmarshal(raw, &node) != nil {
			continue
		}
		if ctxLen := extractCapability(node, ContextLengthKeys); ctxLen > 0 {
			return ctxLen
		}
	}
	return 0
}

// fetchSlotsNctx GETs a /slots endpoint and returns the first positive n_ctx,
// or 0. No auth is set — used by the local provider's own server (no key).
func fetchSlotsNctx(ctx context.Context, slotsURL string, doer HTTPDoer) int {
	ctx, cancel := context.WithTimeout(ctx, slotsProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", slotsURL, nil)
	if err != nil {
		return 0
	}
	resp, err := doer.Do(req)
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
