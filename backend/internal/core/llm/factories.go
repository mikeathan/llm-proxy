package llm

import (
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
)

type ProviderFactory func(cfg models.ModelConfig, manifest models.ProviderManifest) models.Provider

var defaultFactories = map[models.ProviderArchetype]ProviderFactory{
	models.ArchetypeOpenAICompatible: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return providers.NewOpenAICompatibleProvider(cfg, m)
	},
	models.ArchetypeGemini: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return providers.NewGeminiProvider(cfg)
	},
	models.ArchetypeVertex: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return providers.NewVertexProvider(cfg)
	},
}

func GetProviderFactory(archetype models.ProviderArchetype) (ProviderFactory, bool) {
	f, ok := defaultFactories[archetype]
	return f, ok
}
