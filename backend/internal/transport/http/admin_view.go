package api

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/llm/metadata"
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

// getModelsView constructs a view-friendly list of models. For local models that
// have no cached metadata yet, it scans the GGUF header (fast — header-only via
// SkipLargeMetadata+MMap) and persists the result so subsequent page loads are instant.
func (h *AdminHandlers) getModelsView(ctx context.Context, modelsList []models.ModelConfig, activeName string, activeReady bool) []adminModelView {
	out := make([]adminModelView, 0, len(modelsList))
	host := h.runtime.ModelHost()
	scanner := metadata.NewGGUFScanner()

	for _, mc := range modelsList {
		filename := mc.Filename
		if filename == "" && mc.Path != "" {
			filename = filepath.Base(mc.Path)
		}

		fullPath := mc.Path
		if fullPath == "" && mc.Provider == "local" && filename != "" {
			fullPath = h.admin.ResolveModelPath(filename, "")
		}

		meta := mc.Metadata
		if meta == nil && mc.Provider == "local" && fullPath != "" {
			// Header-only scan: reads ~KB not GB, completes in milliseconds.
			if m, err := scanner.Scan(ctx, fullPath); err == nil {
				meta = &m
				// Persist so next page load skips the scan entirely.
				updated := mc
				updated.Metadata = meta
				_ = h.admin.PersistReplaceModel(updated)
			}
		}

		view := adminModelView{
			Name:           mc.Name,
			Provider:       mc.Provider,
			Filename:       filename,
			ResolvedPath:   fullPath,
			Args:           mc.Args,
			Port:           mc.Port,
			Active:         mc.Name == activeName,
			Ready:          mc.Name == activeName && activeReady,
			ProviderConfig: mc.ProviderConfig,
			Metadata:       meta,
			Prefill:        mc.Prefill,
			MaxSteps:       mc.MaxSteps,
			ContextBudget:  mc.ContextBudget,
			ToolCallFormat: mc.ToolCallFormat,
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
