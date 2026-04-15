package providers

import (
	"fmt"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"sync"
)

// ProviderRegistrar manages the configuration and instantiation of LLM providers.
// It centralizes infrastructure settings and secret resolution.
type ProviderRegistrar struct {
	mu        sync.RWMutex
	registry  *ProviderRegistry
	configs   map[string]models.ProviderItem
	secrets   models.SecretsStore
	modelHost string

	// System defaults
	defaultBinary   string
	defaultModelDir string
}

// NewProviderRegistrar creates a new registrar with the given dependencies.
func NewProviderRegistrar(registry *ProviderRegistry, secrets models.SecretsStore, modelHost string) *ProviderRegistrar {
	return &ProviderRegistrar{
		registry:  registry,
		configs:   make(map[string]models.ProviderItem),
		secrets:   secrets,
		modelHost: modelHost,
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

// SetModelHost updates the host used for local model inference.
func (r *ProviderRegistrar) SetModelHost(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelHost = host
}

// Build instantiates a models.Provider based on the provided model configuration.
// It handles parameter hydration and secret resolution.
func (r *ProviderRegistrar) Build(cfg models.ModelConfig) (models.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pCfg := cfg
	providerName := cfg.Provider
	var modelDir string

	// 1. Hydrate Infrastructure Defaults (URLs, IDs, Paths)
	if provider, ok := r.configs[providerName]; ok {
		if pCfg.ProviderConfig.BaseURL == "" {
			pCfg.ProviderConfig.BaseURL = provider.BaseURL
		}
		if pCfg.ProviderConfig.ProjectID == "" {
			pCfg.ProviderConfig.ProjectID = provider.ProjectID
		}
		if pCfg.ProviderConfig.Region == "" {
			pCfg.ProviderConfig.Region = provider.Region
		}
		modelDir = provider.ModelDir
	}

	// 2. Resolve Secrets (API Keys)
	if r.secrets != nil && providerName != "local" {
		apiKey := pCfg.ProviderConfig.APIKey
		apiKeyName := pCfg.ProviderConfig.APIKeyName

		if apiKey == "" || storage.IsMasked(apiKey) {
			// Resolve by name or mask
			realKey, err := r.resolveSecret(providerName, apiKey, apiKeyName)
			if err == nil {
				pCfg.ProviderConfig.APIKey = realKey
			}
		}
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
		return NewLocalProvider(pCfg, binary, modelDir), nil
	}

	// Dynamic Resolution via Manifest Registry
	if manifest, ok := r.registry.Get(providerName); ok {
		if factory, ok := GetProviderFactory(manifest.Archetype); ok {
			return factory(pCfg, manifest), nil
		}
	}

	// Resilient Fallback to local for unknown providers
	return NewLocalProvider(pCfg, r.defaultBinary, modelDir), nil
}

// resolveSecret handles the logic of finding the real key for a masked or empty input.
func (r *ProviderRegistrar) resolveSecret(provider, key, name string) (string, error) {
	if r.secrets == nil {
		return "", fmt.Errorf("secrets store not initialized")
	}

	// 1. Try resolving by explicit name/ID
	if real, err := r.secrets.GetResolvedProviderKey(provider, name); err == nil {
		return real, nil
	}

	// 2. Fallback: try resolving by pattern matching the mask
	if storage.IsMasked(key) {
		if real, err := r.secrets.ResolveMaskedKey(provider, key); err == nil {
			return real, nil
		}
	}

	return "", fmt.Errorf("could not resolve secret for %s", provider)
}

// ModelHost returns the current inference host.
func (r *ProviderRegistrar) ModelHost() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.modelHost == "" {
		return "127.0.0.1"
	}
	return r.modelHost
}

// DefaultBinary returns the configured llama-server binary path.
func (r *ProviderRegistrar) DefaultBinary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
