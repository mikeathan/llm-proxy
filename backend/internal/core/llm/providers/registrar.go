package providers

import (
	"errors"
	"fmt"
	"llm-proxy/internal/core"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"sync"
)

// ErrUnknownProvider is the typed error returned when a model references a
// provider that is not registered.  Providers fail closed rather than
// silently falling back to local — a stale vertex/mulerouter entry in an
// installed registry must be reported, never re-routed to the local host.
var ErrUnknownProvider = errors.New("unknown provider")

// ErrUnresolvedKey is the typed error returned when a cloud model explicitly
// references a credential (by name or masked placeholder) that cannot be
// hydrated to a real key.  Shipping an empty/masked Authorization header would
// only produce a confusing 401 at request time, so Build fails fast instead.
var ErrUnresolvedKey = errors.New("unresolved provider API key")

// ProviderRegistrar manages the configuration and instantiation of LLM providers.
// It centralizes infrastructure settings and secret resolution.
type ProviderRegistrar struct {
	mu        sync.RWMutex
	registry  *ProviderRegistry
	configs   map[string]models.ProviderItem
	secrets   models.SecretsStore
	modelHost string

	// workloadClassifier is built once with the model host and cached local
	// interface IPs; reused for runtime workload classification.
	workloadClassifier models.WorkloadClassifier

	// doer is the dedicated provider infrastructure HTTP client (shared
	// platform transport-backed, 45s timeout).  Injected by bootstrap.
	doer HTTPDoer

	// catalogCache is the SHARED catalog listing cache owned by the registrar.
	// Provider instances are built fresh per call, so the cache must live here
	// (not on the provider) for a catalog fetched once to be reused across
	// calls (§2.10 #3).  Injected into OpenAI-compatible providers at Build.
	catalogCache *core.TTLCache[string, []models.ProviderModelInfo]

	// System defaults
	defaultBinary   string
	defaultModelDir string
}

// NewProviderRegistrar creates a new registrar with the given dependencies.
func NewProviderRegistrar(registry *ProviderRegistry, secrets models.SecretsStore, modelHost string) *ProviderRegistrar {
	return &ProviderRegistrar{
		registry:           registry,
		configs:            make(map[string]models.ProviderItem),
		secrets:            secrets,
		modelHost:          modelHost,
		workloadClassifier: models.NewWorkloadClassifier(modelHost, models.LocalInterfaceIPs()),
		catalogCache:       core.NewTTLCache[string, []models.ProviderModelInfo](catalogCacheMaxEntries, catalogTTL, nil),
	}
}

// RegisterLocal configures the local llama-server infrastructure.
func (r *ProviderRegistrar) RegisterLocal(binary, modelDir string, defaultArgs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.defaultBinary = binary
	r.defaultModelDir = modelDir
	r.configs["local"] = models.ProviderItem{
		Type:              "local",
		LlamaServerBinary: binary,
		ModelDir:          modelDir,
		DefaultArgs:       defaultArgs,
	}
}

// RegisterCloud configures a cloud provider's infrastructure settings.
func (r *ProviderRegistrar) RegisterCloud(name string, item models.ProviderItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[name] = item
}

// SetSecrets updates the secrets store used for credential resolution.
func (r *ProviderRegistrar) SetSecrets(s models.SecretsStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets = s
}

// SetHTTPDoer injects the dedicated provider infrastructure HTTP client
// (shared platform transport-backed, 45s timeout) threaded into provider
// factories at build time.  Providers never inherit agent-tool guardrails (C1).
func (r *ProviderRegistrar) SetHTTPDoer(doer HTTPDoer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doer = doer
}

// SetModelHost updates the host used for local model inference.
func (r *ProviderRegistrar) SetModelHost(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelHost = host
	r.workloadClassifier = models.NewWorkloadClassifier(host, models.LocalInterfaceIPs())
}

// Build instantiates a models.Provider based on the provided model configuration.
// It handles parameter hydration and secret resolution.
func (r *ProviderRegistrar) Build(cfg models.ModelConfig) (models.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pCfg := cfg
	if pCfg.ProviderConfig == nil {
		pCfg.ProviderConfig = &models.ProviderConfig{}
	} else {
		// Deep copy to avoid side effects
		copy := *pCfg.ProviderConfig
		pCfg.ProviderConfig = &copy
	}

	providerName := cfg.Provider
	var modelDir string

	// 1. Resolve Secrets (API Keys) — per-key credentials + base_url override
	if r.secrets != nil && providerName != "local" {
		apiKey := pCfg.ProviderConfig.APIKey
		apiKeyName := pCfg.ProviderConfig.APIKeyName

		if apiKey == "" || storage.IsMasked(apiKey) {
			realKey, keyBaseURL, err := r.resolveSecret(providerName, apiKey, apiKeyName)
			if err != nil {
				// An explicit credential reference (named key or masked
				// placeholder) that cannot be hydrated is always a
				// misconfiguration — fail loudly rather than shipping an
				// empty/masked Authorization header that surfaces only as a
				// confusing 401 at request time.  A wholly unkeyed model
				// (no name, nothing stored) may still be a no-auth endpoint,
				// so that case is tolerated with a warning.
				if apiKeyName != "" || storage.IsMasked(apiKey) {
					return nil, fmt.Errorf("%w: provider %q credential %q", ErrUnresolvedKey, providerName, apiKeyName)
				}
				logging.Warn("cloud provider has no configured key; proceeding without auth",
					"provider", providerName, "credential", apiKeyName)
			} else {
				pCfg.ProviderConfig.APIKey = realKey
				if keyBaseURL != "" {
					pCfg.ProviderConfig.BaseURL = keyBaseURL
					logging.Debug("Applied per-key base_url from secret", "provider", providerName, "url", keyBaseURL)
				}
			}
		}
	}

	// 2. Hydrate Infrastructure Defaults (URLs, IDs, Paths) — only for fields not set by secrets
	if provider, ok := r.configs[providerName]; ok {
		if pCfg.ProviderConfig.BaseURL == "" {
			pCfg.ProviderConfig.BaseURL = provider.BaseURL
			logging.Debug("Hydrated provider BaseURL from registrar config", "provider", providerName, "url", provider.BaseURL)
		}
		if pCfg.ProviderConfig.ProjectID == "" {
			pCfg.ProviderConfig.ProjectID = provider.ProjectID
		}
		if pCfg.ProviderConfig.Region == "" {
			pCfg.ProviderConfig.Region = provider.Region
		}
		if pCfg.Args == nil || len(pCfg.Args) == 0 {
			pCfg.Args = provider.DefaultArgs
		}
		modelDir = provider.ModelDir
	} else {
		logging.Debug("No registrar config found for provider", "provider", providerName)
	}

	// 3. Instantiate Provider
	if providerName == "local" {
		binary := r.defaultBinary
		if local, ok := r.configs["local"]; ok && local.LlamaServerBinary != "" {
			binary = local.LlamaServerBinary
			if modelDir == "" {
				modelDir = local.ModelDir
			}
		}
		return NewLocalProvider(pCfg, binary, modelDir, r.ModelHost()), nil
	}

	// Dynamic Resolution via Manifest Registry
	if manifest, ok := r.registry.Get(providerName); ok {
		logging.Debug("Instantiating cloud provider", "name", providerName, "url", pCfg.ProviderConfig.BaseURL, "manifest_default", manifest.DefaultBaseURL)
		if factory, ok := GetProviderFactory(manifest.Archetype); ok {
			provider := factory(pCfg, manifest)
			// Thread the dedicated infrastructure client + shared workload
			// classifier into providers that accept them (C1, §3.4).
			if settable, ok := provider.(interface{ SetHTTPDoer(HTTPDoer) }); ok {
				settable.SetHTTPDoer(r.doer)
			}
			if settable, ok := provider.(interface{ SetWorkloadClassifier(models.WorkloadClassifier) }); ok {
				settable.SetWorkloadClassifier(r.workloadClassifier)
			}
			if settable, ok := provider.(interface{ SetCatalogCache(*core.TTLCache[string, []models.ProviderModelInfo]) }); ok {
				settable.SetCatalogCache(r.catalogCache)
			}
			return provider, nil
		}
	}

	// Fail closed: an unknown provider must never silently fall back to the
	// local host.  Removing a provider (e.g. vertex/mulerouter deprecation)
	// surfaces affected models as a typed error so the operator can migrate
	// or delete them explicitly.
	return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
}

// resolveSecret handles the logic of finding the real key for a masked or empty input.
// Returns (key, baseURL, error).
func (r *ProviderRegistrar) resolveSecret(provider, key, name string) (string, string, error) {
	if r.secrets == nil {
		return "", "", fmt.Errorf("secrets store not initialized")
	}

	// Use GetResolvedProviderKeyInfo which returns both the key and its base_url
	if info, err := r.secrets.GetResolvedProviderKeyInfo(provider, name); err == nil {
		return info.Key, info.BaseURL, nil
	}

	// Fallback: try resolving by pattern matching the mask
	if storage.IsMasked(key) {
		if real, err := r.secrets.ResolveMaskedKey(provider, key); err == nil {
			return real, "", nil
		}
	}

	return "", "", fmt.Errorf("could not resolve secret for %s", provider)
}

// ModelHost returns the current inference host.
func (r *ProviderRegistrar) ModelHost() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return network.ResolveHost(r.modelHost)
}

// EffectiveEndpoint resolves the fully-hydrated base URL for a model's provider
// endpoint, mirroring Build()'s secret-hydration and infrastructure-default
// steps (1–2) WITHOUT constructing a provider.  This is the same source used
// for inference, so workload classification and budget/context resolution key
// on the effective endpoint — including per-credential base-URL overrides.
// Returns an empty string when nothing is configured (local models).
func (r *ProviderRegistrar) EffectiveEndpoint(cfg models.ModelConfig) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.EffectiveEndpointLocked(cfg)
}

// EffectiveEndpointLocked is the lock-free core of EffectiveEndpoint.  Callers
// must hold r.mu (read).
func (r *ProviderRegistrar) EffectiveEndpointLocked(cfg models.ModelConfig) string {
	pCfg := cfg.ProviderConfig
	if pCfg == nil {
		return ""
	}

	providerName := cfg.Provider
	if providerName == "" || providerName == "local" {
		return ""
	}

	baseURL := pCfg.BaseURL

	// 1. Per-credential base_url override from the secrets store.
	if r.secrets != nil {
		apiKey := pCfg.APIKey
		apiKeyName := pCfg.APIKeyName
		if apiKey == "" || storage.IsMasked(apiKey) {
			if _, keyBaseURL, err := r.resolveSecret(providerName, apiKey, apiKeyName); err == nil && keyBaseURL != "" {
				baseURL = keyBaseURL
			}
		}
	}

	// 2. Registrar infrastructure default.
	if baseURL == "" {
		if item, ok := r.configs[providerName]; ok {
			baseURL = item.BaseURL
		}
	}

	// 3. Manifest default base URL (no configured override).
	if baseURL == "" {
		if manifest, ok := r.registry.Get(providerName); ok {
			baseURL = manifest.DefaultBaseURL
		}
	}

	return baseURL
}

// Classify returns the workload class for a model using the fully-hydrated
// effective endpoint (per-credential overrides + provider defaults applied).
func (r *ProviderRegistrar) Classify(cfg models.ModelConfig) models.WorkloadClass {
	r.mu.RLock()
	defer r.mu.RUnlock()
	classifierCfg := cfg
	if classifierCfg.ProviderConfig == nil {
		classifierCfg.ProviderConfig = &models.ProviderConfig{}
	} else {
		cp := *classifierCfg.ProviderConfig
		classifierCfg.ProviderConfig = &cp
	}
	classifierCfg.ProviderConfig.BaseURL = r.EffectiveEndpointLocked(cfg)
	return r.workloadClassifier.Classify(classifierCfg)
}

// DefaultBinary returns the configured llama-server binary path.

// ResolveBinary returns the best available binary path: the explicit value from
// the local provider config if set, otherwise the default. This is the
// authoritative resolution used both at model start and on config updates.
func (r *ProviderRegistrar) ResolveBinary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if local, ok := r.configs["local"]; ok && local.LlamaServerBinary != "" {
		return local.LlamaServerBinary
	}
	return r.defaultBinary
}

// ListConfigs returns a copy of the currently registered provider configurations.
func (r *ProviderRegistrar) ListConfigs() map[string]models.ProviderItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]models.ProviderItem, len(r.configs))
	for k, v := range r.configs {
		out[k] = v
	}
	return out
}
