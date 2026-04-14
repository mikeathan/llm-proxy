package providers

import (
	"llm-proxy/models"
)

type ProviderFactory func(cfg models.ModelConfig, manifest models.ProviderManifest) models.Provider

var defaultFactories = map[models.ProviderArchetype]ProviderFactory{
	models.ArchetypeOpenAICompatible: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return NewOpenAICompatibleProvider(cfg, m)
	},
	models.ArchetypeGemini: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return NewGeminiProvider(cfg)
	},
	models.ArchetypeVertex: func(cfg models.ModelConfig, m models.ProviderManifest) models.Provider {
		return NewVertexProvider(cfg)
	},
}

func GetProviderFactory(archetype models.ProviderArchetype) (ProviderFactory, bool) {
	f, ok := defaultFactories[archetype]
	return f, ok
}
