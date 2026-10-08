package llm

import (
	"context"
	"time"

	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// servingFingerprintTimeout bounds one model's listing read at startup.
const servingFingerprintTimeout = 5 * time.Second

// RefreshServingFingerprints reads each OpenAI-style model's own entry in its
// provider's live listing and, when the entry identifies a llama.cpp server,
// records that on the model and re-derives its workload class and budgets. It
// is how a model saved before the listing carried the fingerprint (or one whose
// remote server was down at discovery) becomes a local workload without being
// re-added. Best-effort and runtime-only: an unreachable server or an entry
// without the fingerprint changes nothing, and the next start reads it again.
// Cloud catalogs never carry the fingerprint, so they are never reclassified.
func (m *LLMRuntimeManager) RefreshServingFingerprints(ctx context.Context) {
	retryUntilAnswered(ctx, fingerprintRetryDelays, m.fingerprintCandidates(), func(cfg models.ModelConfig) bool {
		return m.refreshServingFingerprint(ctx, cfg)
	})
}

// fingerprintRetryDelays spaces the retries for a server that did not answer
// (still starting, or the network not up yet): a model left unmarked is run as a
// cloud workload for the whole process, so one failed read at startup is worth
// a few more tries. A variable so tests can shorten it.
var fingerprintRetryDelays = []time.Duration{0, 10 * time.Second, time.Minute, 5 * time.Minute}

// retryUntilAnswered calls try for each model after each delay, keeping only the
// models whose try reported no answer for the next round. It stops when none are
// left, the delays run out, or ctx ends.
func retryUntilAnswered(ctx context.Context, delays []time.Duration, candidates []models.ModelConfig, try func(models.ModelConfig) bool) {
	pending := candidates
	for _, delay := range delays {
		if len(pending) == 0 {
			return
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		var unanswered []models.ModelConfig
		for _, cfg := range pending {
			if ctx.Err() != nil {
				return
			}
			if !try(cfg) {
				unanswered = append(unanswered, cfg)
			}
		}
		pending = unanswered
	}
}

// fingerprintCandidates lists the models worth a listing read: OpenAI-style
// models still classified cloud that have no fingerprint yet.
func (m *LLMRuntimeManager) fingerprintCandidates() []models.ModelConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.ModelConfig
	for _, cfg := range m.models {
		if cfg.Provider != models.ProviderOpenAI || cfg.WorkloadClass == models.WorkloadLocal {
			continue
		}
		if cfg.Metadata != nil && cfg.Metadata.Serving != "" {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

// refreshServingFingerprint reads one model's listing entry and applies its
// fingerprint. It reports whether the server answered (whether or not the entry
// carried a fingerprint), so the caller can retry only the unreachable ones.
func (m *LLMRuntimeManager) refreshServingFingerprint(ctx context.Context, cfg models.ModelConfig) bool {
	p, err := m.registrar.Build(cfg)
	if err != nil {
		return false
	}
	listCtx, cancel := context.WithTimeout(ctx, servingFingerprintTimeout)
	defer cancel()
	infos, err := p.ListModels(listCtx)
	if err != nil {
		return false
	}
	for _, info := range infos {
		if info.ID == cfg.Filename {
			m.applyServingFingerprint(cfg.Name, info)
			break
		}
	}
	return true
}

func (m *LLMRuntimeManager) applyServingFingerprint(name string, info models.ProviderModelInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.models[name]
	if !ok {
		return
	}
	updated, changed := withServingFingerprint(cur, info, m.registrar.Classify)
	if !changed {
		return
	}
	if override, ok := m.overrides[name]; ok {
		m.applyOverride(&updated, name, override)
	}
	m.models[name] = updated
	logging.Info("model identified as a llama.cpp server; treating it as a local workload",
		"model", name, "serving_ctx", updated.Metadata.Nctx,
		"max_tokens", updated.MaxTokens, "context_budget", updated.ContextBudget)
}

// withServingFingerprint applies a listing entry's llama.cpp fingerprint to cfg:
// it records it, adopts the entry's serving context, and re-derives the workload
// class and budgets (a local workload's budgets come from the serving context,
// not from the cloud policy). It reports whether anything changed.
func withServingFingerprint(cfg models.ModelConfig, info models.ProviderModelInfo, classify func(models.ModelConfig) models.WorkloadClass) (models.ModelConfig, bool) {
	if info.Meta == nil || info.Meta.Serving == "" {
		return cfg, false
	}
	if cfg.Metadata != nil && cfg.Metadata.Serving == info.Meta.Serving {
		return cfg, false
	}
	md := models.ModelMetadata{}
	if cfg.Metadata != nil {
		md = *cfg.Metadata
	}
	md.Serving = info.Meta.Serving
	if info.Meta.Nctx > 0 {
		md.Nctx = info.Meta.Nctx
	}
	cfg.Metadata = &md
	cfg.WorkloadClass = classify(cfg)
	cfg.MaxTokens = 0
	cfg.ContextBudget = 0
	// "native" was the cloud default for an unset format; a local workload
	// probes its format (an explicit override is re-applied by the caller).
	cfg.ToolCallFormat = ""
	orchestrator.ApplyMetadataDefaults(&cfg)
	return cfg, true
}
