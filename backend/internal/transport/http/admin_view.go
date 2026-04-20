package api

import (
	"fmt"
	"llm-proxy/models"
	"path/filepath"
)

// getProvidersView constructs a unified view of all providers (Registry + Local)
// and enriches them with masked API keys from the secrets store.
func (h *AdminHandlers) getProvidersView() map[string]adminProviderView {
	allProviders := h.admin.Providers()
	secrets := h.admin.Secrets()
	out := make(map[string]adminProviderView)

	for id, p := range allProviders {
		view := adminProviderView{
			ProviderItem: p,
		}
		if secrets != nil {
			view.APIKeys = secrets.MaskedProviderKeys(id)
		}
		out[id] = view
	}

	return out
}

// getModelsView constructs a view-friendly list of models, handling path resolution
// for local providers and ensuring consistent identifier mapping.
func (h *AdminHandlers) getModelsView(modelsList []models.ModelConfig, activeName string, activeReady bool) []adminModelView {
	out := make([]adminModelView, 0, len(modelsList))
	host := h.runtime.ModelHost()

	for _, mc := range modelsList {
		filename := mc.Filename
		if filename == "" && mc.Path != "" {
			filename = filepath.Base(mc.Path)
		}

		view := adminModelView{
			Name:           mc.Name,
			Provider:       mc.Provider,
			Filename:       filename,
			ResolvedPath:   mc.Path,
			Args:           mc.Args,
			Port:           mc.Port,
			Active:         mc.Name == activeName,
			Ready:          mc.Name == activeName && activeReady,
			ProviderConfig: mc.ProviderConfig,
		}

		if mc.Port > 0 {
			view.Endpoint = fmt.Sprintf("http://%s:%d", host, mc.Port)
		}

		out = append(out, view)
	}
	return out
}

func translateProvidersToRegistry(in map[string]adminProviderView) map[string]models.ProviderRegistryEntry {
	out := make(map[string]models.ProviderRegistryEntry)
	for id, p := range in {
		if id == "local" {
			continue // Infrastructure-only
		}
		out[id] = models.ProviderRegistryEntry{
			Type:    p.Type,
			BaseURL: p.BaseURL,
		}
	}
	return out
}
