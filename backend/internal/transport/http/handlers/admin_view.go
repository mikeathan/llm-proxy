package handlers

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/llm/metadata"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/network"
	"llm-proxy/models"
	"path/filepath"
)

// modelViewTuning computes the effective agent-tuning values for a single model.
// Zero values from settings.yml overrides are replaced with runtime defaults so
// the frontend always sees the actual effective value, not a raw zero.
func modelViewTuning(mc models.ModelConfig) (prefill bool, maxSteps, contextBudget, maxTokens int, temperature float64, reasoningBudget, slotTimeout, timeoutMinutes int, toolCallFormat, loopStrategy string) {
	prefill = mc.Prefill != nil && *mc.Prefill
	maxSteps = mc.MaxSteps
	if maxSteps == 0 {
		maxSteps = assistant.DefaultMaxSteps
	}
	contextBudget = mc.ContextBudget
	if contextBudget == 0 {
		contextBudget = assistant.DefaultContextBudget
	}
	maxTokens = mc.MaxTokens
	if maxTokens == 0 {
		maxTokens = assistant.DefaultMaxTokens
	}
	temperature = mc.Temperature
	if temperature == 0 {
		temperature = assistant.DefaultAutomationTemperature
	}
	reasoningBudget = mc.ReasoningBudget
	if reasoningBudget == 0 {
		reasoningBudget = assistant.DefaultReasoningBudget(maxTokens)
	}
	slotTimeout = mc.SlotTimeout
	timeoutMinutes = mc.TimeoutMinutes
	if timeoutMinutes == 0 {
		timeoutMinutes = int(assistant.AgentGlobalTimeout.Minutes())
	}
	toolCallFormat = mc.ToolCallFormat
	// loop_strategy defaults to "" (react) — the resolver applies the same
	// default, so the UI shows the provider-default option, not a fabricated
	// value.
	loopStrategy = string(mc.LoopStrategy)
	return
}

// getProvidersView constructs a unified view of all providers (Registry + Local)
// and enriches them with masked API keys from the secrets store.
func getProvidersView(admin AdminService) map[string]adminProviderView {
	allProviders := admin.Providers()
	secrets := admin.Secrets()
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
func getModelsView(ctx context.Context, modelsList []models.ModelConfig, activeName string, activeReady bool, host string, admin AdminService) []adminModelView {
	out := make([]adminModelView, 0, len(modelsList))
	scanner := metadata.NewGGUFScanner()

	for _, mc := range modelsList {
		filename := mc.Filename
		if filename == "" && mc.Path != "" {
			filename = filepath.Base(mc.Path)
		}

		fullPath := mc.Path
		if fullPath == "" && mc.Provider == "local" && filename != "" {
			fullPath = admin.ResolveModelPath(filename, "")
		}

		meta := mc.Metadata
		if meta == nil && mc.Provider == "local" && fullPath != "" {
			if m, err := scanner.Scan(ctx, fullPath); err == nil {
				meta = &m
				updated := mc
				updated.Metadata = meta
				orchestrator.ApplyMetadataDefaults(&updated)
				_ = admin.PersistReplaceModel(updated)
			}
		}

		prefill, maxSteps, contextBudget, maxTokens, temperature, reasoningBudget, slotTimeout, timeoutMinutes, toolCallFormat, loopStrategy := modelViewTuning(mc)

		view := adminModelView{
			Name:             mc.Name,
			Provider:         mc.Provider,
			WorkloadClass:    mc.WorkloadClass,
			Filename:         filename,
			ResolvedPath:     fullPath,
			Args:             mc.Args,
			Port:             mc.Port,
			Active:           mc.Name == activeName,
			Ready:            mc.Name == activeName && activeReady,
			ProviderConfig:   mc.ProviderConfig,
			Metadata:         meta,
			Prefill:          prefill,
			MaxSteps:         maxSteps,
			ContextBudget:    contextBudget,
			MaxTokens:        maxTokens,
			Temperature:      temperature,
			ReasoningBudget:  reasoningBudget,
			ReasoningEnabled: mc.ReasoningEnabled,
			SlotTimeout:      slotTimeout,
			TimeoutMinutes:   timeoutMinutes,
			ToolCallFormat:   toolCallFormat,
			LoopStrategy:     loopStrategy,
		}

		if mc.Port > 0 {
			view.Endpoint = network.FormatURL(host, mc.Port)
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
