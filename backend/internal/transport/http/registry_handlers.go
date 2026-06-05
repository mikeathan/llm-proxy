package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/llm/metadata"
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"os"
)

func (h *AdminHandlers) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleAddModel(w, r)
}

func (h *AdminHandlers) AdminUpdateModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateModel(w, r)
}

func (h *AdminHandlers) AdminDeleteModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteModel(w, r)
}

func (h *AdminHandlers) AdminDeleteAllModelsHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteAllModels(w, r)
}

// AdminRegistryHandler handles GET /admin/api/registry
func (h *AdminHandlers) AdminRegistryHandler(w http.ResponseWriter, r *http.Request) {
	reg := h.admin.GetRegistry()
	view := adminRegistryView{
		Catalogue:  reg.Catalogue,
		Providers:  h.getProvidersView(),
		MCPServers: reg.MCPServers,
	}
	respondJSON(w, view)
}

// AdminRegistryPutHandler handles PUT /admin/api/registry
func (h *AdminHandlers) AdminRegistryPutHandler(w http.ResponseWriter, r *http.Request) {
	var req adminRegistryView
	if !decodeJSONBody(w, r, &req) {
		return
	}

	err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		reg.Catalogue = req.Catalogue
		reg.Providers = translateProvidersToRegistry(req.Providers)
		reg.MCPServers = req.MCPServers
	})

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update registry: "+err.Error())
		return
	}

	// Trigger immediate sync to refresh provider settings and model configs
	h.runtime.Sync()
	h.runtime.ApplyModelOverrides(h.admin.GetSettings().ModelOverrides)

	respondJSON(w, h.admin.GetRegistry())
}

// MCP Handlers

func (h *AdminHandlers) AdminMCPListHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.ListMCPServers())
}

func (h *AdminHandlers) AdminMCPAddHandler(w http.ResponseWriter, r *http.Request) {
	var req models.MCPServerConfig
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name == "" || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	if err := h.admin.AddMCPServer(req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to add mcp server: "+err.Error())
		return
	}

	respondJSON(w, req)
}

func (h *AdminHandlers) AdminMCPUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.MCPServerConfig
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.admin.UpdateMCPServer(req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update mcp server: "+err.Error())
		return
	}

	respondJSON(w, req)
}

func (h *AdminHandlers) AdminMCPRemoveHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing name")
		return
	}

	if err := h.admin.RemoveMCPServer(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to remove mcp server: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandlers) AdminListProviderManifestsHandler(w http.ResponseWriter, r *http.Request) {
	manifests := providers.GetRegistry().List()
	respondJSON(w, manifests)
}

func (h *AdminHandlers) AdminListProviderModelsHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	apiKeyName := r.URL.Query().Get("api_key_name")
	infos, err := h.runtime.ListProviderModels(r.Context(), provider, apiKeyName)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list models: "+err.Error())
		return
	}

	respondJSON(w, infos)
}

func (h *AdminHandlers) AdminTestProviderConnectionHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	apiKey := r.URL.Query().Get("api_key")
	apiKeyName := r.URL.Query().Get("api_key_name")
	baseURL := r.URL.Query().Get("base_url")

	err := h.runtime.TestProviderConnection(r.Context(), provider, apiKey, apiKeyName, baseURL)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "connection test failed: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok", "message": "Connection successful"})
}

// modelFormRequest is the shared payload for both add- and update-model
// handlers.  The same struct is used for registration and editing so the
// metadata enrichment and override-save logic stays in one place.
type modelFormRequest struct {
	Name                 string                `json:"name"`
	Provider             string                `json:"provider"`
	Filename             string                `json:"filename"`
	ModelID              string                `json:"model_id"`
	Path                 string                `json:"path"`
	Args                 []string              `json:"args"`
	Port                 int                   `json:"port"`
	ProviderConfig       models.ProviderConfig `json:"provider_config"`
	Metadata             *models.ModelMetadata `json:"metadata"`
	Prefill              *bool                 `json:"prefill,omitempty"`
	MaxSteps             int                   `json:"max_steps"`
	ContextBudget        int                   `json:"context_budget"`
	MaxTokens            int                   `json:"max_tokens"`
	ReasoningBudget      int                   `json:"reasoning_budget"`
	SlotTimeout          int                   `json:"slot_timeout"`
	ToolCallFormat       string                `json:"tool_call_format"`
	Pricing              *models.ModelPricing  `json:"pricing"`
	Limits               *models.ModelLimits   `json:"limits"`
	Meta                 *models.ModelMeta     `json:"meta"`
}

// enrichMetadataFromProviders populates model metadata from the provider's
// model-listing API (meta, limits) and auto-computes InternalCreditWeight
// from OpenRouter pricing.  When exact context length is available, it
// zeroes the provider-tier defaults so ApplyMetadataDefaults can derive
// correct max_tokens/context_budget from the model's actual capabilities.
func (r *modelFormRequest) enrichMetadataFromProviders() {
	logging.Debug("[enrich] input",
		"meta", r.Meta, "limits", r.Limits, "pricing", r.Pricing,
		"max_tokens", r.MaxTokens, "ctx_budget", r.ContextBudget)

	if r.Provider != "local" && r.Pricing != nil && r.ProviderConfig.InternalCreditWeight <= 0 {
		r.ProviderConfig.InternalCreditWeight = orchestrator.ComputeICUWeightFromPricing(r.Pricing)
	}
	if r.Metadata == nil && r.Limits != nil && r.Limits.Context > 0 {
		r.Metadata = &models.ModelMetadata{ContextLength: r.Limits.Context}
		logging.Debug("[enrich] metadata from limits", "context", r.Limits.Context)
	}

	// Resolve context length from model meta.  Prefer n_ctx (serving
	// context) over n_ctx_train (training context).  n_ctx_train values
	// above 128K are never serving contexts for current cloud models
	// and indicate a local llama.cpp where training context dwarfs the
	// actual --ctx-size.  When only n_ctx_train is available and it
	// exceeds 128K, keep the provider-tier defaults.
	metaCtx := 0
	if r.Meta != nil {
		if r.Meta.Nctx > 0 {
			metaCtx = r.Meta.Nctx
			logging.Debug("[enrich] using n_ctx (serving context)", "ctx", metaCtx)
		} else if r.Meta.ContextLength > 0 && r.Meta.ContextLength <= 128_000 {
			metaCtx = r.Meta.ContextLength
			logging.Debug("[enrich] using n_ctx_train", "ctx", metaCtx)
		} else if r.Meta.ContextLength > 128_000 {
			logging.Warn("[enrich] n_ctx_train too large, keeping tier defaults",
				"n_ctx_train", r.Meta.ContextLength,
				"provider", r.Provider)
		}
	}

	if r.Metadata == nil && metaCtx > 0 {
		r.Metadata = &models.ModelMetadata{ContextLength: r.Meta.ContextLength, Nctx: metaCtx, Parameters: r.Meta.Parameters}
	}
	if r.Metadata != nil && metaCtx > 0 {
		r.Metadata.Nctx = metaCtx
		r.Metadata.Parameters = r.Meta.Parameters
	}
	if r.Metadata != nil && r.Metadata.ContextLength > 0 {
		logging.Info("[enrich] zeroing tier defaults for metadata-driven recomputation",
			"ctx_len", r.Metadata.ContextLength,
			"original_max_tokens", r.MaxTokens,
			"original_ctx_budget", r.ContextBudget)
		// Zero ALL tier defaults when we have trusted metadata to
		// recompute from.  The threshold-based approach (only zero
		// if below a threshold like <=50000) breaks when the frontend
		// pre-fills the form with values derived from n_ctx_train
		// (e.g. 524288 from 262144 training context), because those
		// exceed the threshold and never get recomputed.
		r.MaxTokens = 0
		r.ContextBudget = 0
		r.ReasoningBudget = 0
	} else if r.Meta != nil && metaCtx <= 0 {
		// Meta was sent by the frontend but we couldn't extract a
		// reliable context length (/slots unreachable, n_ctx_train
		// inflated >128K).  Still zero the tier defaults so
		// ApplyMetadataDefaults recomputes from provider defaults
		// instead of persisting the form's pre-filled values.
		logging.Warn("[enrich] unreliable metadata, zeroing tier defaults",
			"meta", r.Meta)
		r.MaxTokens = 0
		r.ContextBudget = 0
		r.ReasoningBudget = 0
	} else {
		logging.Info("[enrich] no context length available, keeping tier defaults")
	}
}

// writeModelOverrides persists agent-tuning fields to settings.yml
// (model_overrides) when any non-zero value is present.  It uses the
// computed ModelConfig (post-ApplyMetadataDefaults) rather than the raw
// form request, because enrichMetadataFromProviders zeros the form values
// and the recomputed values must replace any stale overrides.
//
// Zero values are silently skipped so per-model defaults stay active
// until the user explicitly changes them.
//
// MaxTokens and ContextBudget are deliberately excluded — they are
// metadata-derived values that ApplyMetadataDefaults recomputes from the
// model's context length on every startup.  Persisting them would freeze
// stale values that override future computation.

// hasModelOverrides returns true when the model config contains at least one
// non-zero agent-tuning field that should be persisted as a user override.
// Computed fields (MaxTokens, ContextBudget) are excluded — they are
// derived from metadata and must not be frozen in settings.yml.
func hasModelOverrides(cfg models.ModelConfig) bool {
	return cfg.MaxSteps > 0 || cfg.ReasoningBudget > 0 || cfg.SlotTimeout > 0 ||
		cfg.ToolCallFormat != "" || (cfg.Prefill != nil && *cfg.Prefill) || cfg.TimeoutMinutes > 0 ||
		(cfg.ProviderConfig != nil && cfg.ProviderConfig.InternalCreditWeight > 0)
}

func writeModelOverrides(name string, cfg models.ModelConfig, updateFn func(func(*models.UserSettings)) error) {
	if hasModelOverrides(cfg) {
		_ = updateFn(func(s *models.UserSettings) {
			if s.ModelOverrides == nil {
				s.ModelOverrides = make(map[string]models.ModelOverride)
			}
			weight := float64(0)
			if cfg.ProviderConfig != nil {
				weight = cfg.ProviderConfig.InternalCreditWeight
			}
			s.ModelOverrides[name] = models.ModelOverride{
				MaxSteps:        cfg.MaxSteps,
				ReasoningBudget: cfg.ReasoningBudget,
				SlotTimeout:     cfg.SlotTimeout,
				ICUWeight:       weight,
				ToolCallFormat:  cfg.ToolCallFormat,
				Prefill:         cfg.Prefill,
				TimeoutMinutes:  cfg.TimeoutMinutes,
			}
		})
	}
}

// handleAddModel registers a new model in the runtime and persists it.
// For OpenRouter (and similar providers that return pricing), it
// auto-computes InternalCreditWeight from the pricing data so ICU weight
// is accurate without manual configuration.
func (h *AdminHandlers) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req modelFormRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	logging.Info("[addModel] received", "name", req.Name, "provider", req.Provider,
		"meta", req.Meta, "limits", req.Limits, "pricing", req.Pricing,
		"max_steps", req.MaxSteps, "ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens)

	if req.Provider == "" {
		req.Provider = "local"
	}

	filename := strings.TrimSpace(req.Filename)
	if filename == "" && req.Path != "" {
		filename = filepath.Base(req.Path)
	}
	if filename == "" && req.ModelID != "" {
		filename = strings.TrimSpace(req.ModelID)
	}
	if filename == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model identifier (filename or model_id)")
		return
	}

	if req.Name == "" {
		ext := filepath.Ext(filename)
		req.Name = strings.TrimSuffix(filename, ext)
	}

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(filename, req.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeJSONError(w, http.StatusBadRequest, "model file not found")
			return
		}
	}

	if req.Port == 0 && req.Provider == "local" {
		active := h.runtime.ActiveInfo()
		activePort := 0
		if active != nil {
			activePort = active.Port
		}
		req.Port = nextAvailablePort(h.runtime.ListModels(), activePort)
	}

	var runtimeArgs []string
	if len(req.Args) == 0 {
		settings := h.admin.GetSettings()
		runtimeArgs = append([]string(nil), settings.Local.DefaultArgs...)
	} else {
		runtimeArgs = append([]string(nil), req.Args...)
	}

	req.enrichMetadataFromProviders()

	logging.Debug("[addModel] before ApplyMetadataDefaults", "ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens, "reasoning", req.ReasoningBudget, "metadata", req.Metadata)
	runtimeCfg := models.ModelConfig{
		Name:             req.Name,
		Provider:         req.Provider,
		Filename:         filename,
		Path:             fullPath,
		Args:             runtimeArgs,
		Port:             req.Port,
		Environment:      h.admin.Environment(),
		ProviderConfig:   &req.ProviderConfig,
		Metadata:         req.Metadata,
		Prefill:          req.Prefill,
		MaxSteps:         req.MaxSteps,
		ContextBudget:    req.ContextBudget,
		MaxTokens:        req.MaxTokens,
		ReasoningBudget:  req.ReasoningBudget,
		SlotTimeout:      req.SlotTimeout,
		ToolCallFormat:   req.ToolCallFormat,
	}
	orchestrator.ApplyMetadataDefaults(&runtimeCfg)
	logging.Info("[addModel] after ApplyMetadataDefaults", "ctx_budget", runtimeCfg.ContextBudget, "max_tokens", runtimeCfg.MaxTokens, "reasoning", runtimeCfg.ReasoningBudget)

	if err := h.runtime.AddModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrModelExists) {
			status = http.StatusConflict
		}
		writeJSONError(w, status, "unable to add model: "+err.Error())
		return
	}

	persistCfg := models.ModelConfig{
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       filename,
		Args:           append([]string{}, req.Args...),
		Port:           req.Port,
		ProviderConfig: &req.ProviderConfig,
		Metadata:       req.Metadata,
		Prefill:        req.Prefill,
		MaxSteps:       req.MaxSteps,
		ContextBudget:  req.ContextBudget,
		MaxTokens:      req.MaxTokens,
		ReasoningBudget: req.ReasoningBudget,
		SlotTimeout:    req.SlotTimeout,
		ToolCallFormat: req.ToolCallFormat,
	}
	logging.Debug("[addModel] persist ApplyMetadataDefaults", "input_budget", req.ContextBudget)
	orchestrator.ApplyMetadataDefaults(&persistCfg)
	logging.Debug("[addModel] persist result", "output_budget", persistCfg.ContextBudget, "output_max_tokens", persistCfg.MaxTokens)
	if err := h.admin.PersistModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "saved model but failed to persist config: "+err.Error())
		return
	}

	writeModelOverrides(req.Name, runtimeCfg, h.admin.UpdateSettings)

	respondJSON(w, runtimeCfg)
}

// handleUpdateModel updates an existing model in both runtime and
// registry persistence.  Like handleAddModel, it auto-computes ICU weight
// from pricing data when available.
func (h *AdminHandlers) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req modelFormRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	logging.Info("[updateModel] body decoded", "name", req.Name, "provider", req.Provider,
		"meta", req.Meta, "limits", req.Limits, "pricing", req.Pricing,
		"ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens, "reasoning", req.ReasoningBudget)

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	var existing models.ModelConfig
	found := false
	for _, m := range h.runtime.ListModels() {
		if m.Name == req.Name {
			existing = m
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown model")
		return
	}

	if req.Provider == "" {
		req.Provider = existing.Provider
	}
	if req.Provider == "" {
		req.Provider = "local"
	}
	if req.Filename == "" && req.Path != "" {
		req.Filename = filepath.Base(req.Path)
	}
	if req.Filename == "" && req.ModelID != "" {
		req.Filename = strings.TrimSpace(req.ModelID)
	}
	if req.Filename == "" {
		req.Filename = existing.Filename
	}
	if req.Port == 0 && req.Provider == "local" {
		req.Port = existing.Port
	}
	if req.Metadata == nil {
		req.Metadata = existing.Metadata
	}

	var runtimeArgs []string
	if len(req.Args) == 0 {
		runtimeArgs = existing.Args
	} else {
		runtimeArgs = append([]string(nil), req.Args...)
	}

	req.enrichMetadataFromProviders()

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(req.Filename, req.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeJSONError(w, http.StatusBadRequest, "model file not found")
			return
		}
	}

	runtimeCfg := models.ModelConfig{
		Name:             req.Name,
		Provider:         req.Provider,
		Filename:         req.Filename,
		Path:             fullPath,
		Args:             runtimeArgs,
		Port:             req.Port,
		Environment:      h.admin.Environment(),
		ProviderConfig:   &req.ProviderConfig,
		Metadata:         req.Metadata,
		Prefill:          req.Prefill,
		MaxSteps:         req.MaxSteps,
		ContextBudget:    req.ContextBudget,
		MaxTokens:        req.MaxTokens,
		ReasoningBudget:  req.ReasoningBudget,
		SlotTimeout:      req.SlotTimeout,
		ToolCallFormat:   req.ToolCallFormat,
	}
	logging.Debug("[updateModel] before runtime ApplyMetadataDefaults", "runtime_before", runtimeCfg.ContextBudget)
	orchestrator.ApplyMetadataDefaults(&runtimeCfg)
	logging.Info("[updateModel] after runtime ApplyMetadataDefaults", "runtime_after", runtimeCfg.ContextBudget)
	if err := h.runtime.UpdateModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to update model: "+err.Error())
		return
	}

	persistCfg := models.ModelConfig{
		Name:             req.Name,
		Provider:         req.Provider,
		Filename:         req.Filename,
		Args:             append([]string{}, req.Args...),
		Port:             req.Port,
		ProviderConfig:   &req.ProviderConfig,
		Metadata:         req.Metadata,
		Prefill:          req.Prefill,
		MaxSteps:         req.MaxSteps,
		ContextBudget:    req.ContextBudget,
		MaxTokens:        req.MaxTokens,
		ReasoningBudget:  req.ReasoningBudget,
		SlotTimeout:      req.SlotTimeout,
		ToolCallFormat:   req.ToolCallFormat,
	}
	logging.Debug("[updateModel] persist ApplyMetadataDefaults", "input_budget", req.ContextBudget)
	orchestrator.ApplyMetadataDefaults(&persistCfg)
	logging.Info("[updateModel] persist result", "output_budget", persistCfg.ContextBudget, "output_max_tokens", persistCfg.MaxTokens)
	if err := h.admin.PersistReplaceModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "updated model but failed to persist config: "+err.Error())
		return
	}

	writeModelOverrides(req.Name, runtimeCfg, h.admin.UpdateSettings)

	respondJSON(w, runtimeCfg)
}

func (h *AdminHandlers) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		var req struct {
			Name string `json:"name"`
		}
		if r.Header.Get("Content-Type") == "application/json" {
			if !decodeJSONBody(w, r, &req) {
				return
			}
		}
		name = req.Name
	}

	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	if err := h.runtime.RemoveModel(name); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to delete model: "+err.Error())
		return
	}

	if err := h.admin.PersistDeleteModel(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "deleted model but failed to persist config: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandlers) handleDeleteAllModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	allModels := h.runtime.ListModels()
	for _, m := range allModels {
		if m.Provider == provider {
			if err := h.runtime.RemoveModel(m.Name); err != nil {
				logging.Error("Failed to remove model during bulk delete", "name", m.Name, "error", err)
			}
			if err := h.admin.PersistDeleteModel(m.Name); err != nil {
				logging.Error("Failed to persist delete during bulk delete", "name", m.Name, "error", err)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func discoverModelFiles(ctx context.Context, modelDir string, current []models.ModelConfig) []adminAvailableModel {
	if modelDir == "" {
		return nil
	}

	if info, err := os.Stat(modelDir); err != nil || !info.IsDir() {
		return nil
	}

	seenNames := make(map[string]struct{}, len(current))
	seenPaths := make(map[string]struct{}, len(current))
	for _, m := range current {
		seenNames[m.Name] = struct{}{}
		if m.Path != "" {
			seenPaths[m.Path] = struct{}{}
		}
	}

	scanner := metadata.NewGGUFScanner()
	var found []adminAvailableModel
	_ = filepath.WalkDir(modelDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".gguf" {
			return nil
		}
		fullPath := path
		if _, ok := seenPaths[fullPath]; ok {
			return nil
		}

		// Use native GGUF metadata parsing
		meta, err := scanner.Scan(ctx, fullPath)
		if err != nil {
			// Fallback to filename-based name if parsing fails
			meta.Name = strings.TrimSuffix(d.Name(), ext)
		}

		if _, ok := seenNames[meta.Name]; ok {
			return nil
		}

		var sizeBytes int64
		if targetInfo, err := os.Stat(fullPath); err == nil {
			sizeBytes = targetInfo.Size()
		} else if info, err := d.Info(); err == nil {
			sizeBytes = info.Size()
		}

		found = append(found, adminAvailableModel{
			Name:         meta.Name,
			Filename:     d.Name(),
			ResolvedPath: fullPath,
			SizeBytes:    sizeBytes,
			Metadata:     meta,
		})
		return nil
	})

	sort.Slice(found, func(i, j int) bool {
		return found[i].Name < found[j].Name
	})

	return found
}

func nextAvailablePort(modelsList []models.ModelConfig, activePort int) int {
	if activePort != 0 {
		return activePort
	}
	port := 8081
	for _, m := range modelsList {
		if m.Port >= port {
			port = m.Port + 1
		}
	}
	return port
}
