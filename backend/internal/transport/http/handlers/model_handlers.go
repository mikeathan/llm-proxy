package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// ModelHandlers serves model CRUD and provider interaction endpoints.
type ModelHandlers struct {
	runtime RuntimeService
	admin   AdminService
}

func NewModelHandlers(runtime RuntimeService, admin AdminService) *ModelHandlers {
	return &ModelHandlers{runtime: runtime, admin: admin}
}

func (h *ModelHandlers) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleAddModel(w, r)
}

func (h *ModelHandlers) AdminUpdateModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateModel(w, r)
}

func (h *ModelHandlers) AdminDeleteModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteModel(w, r)
}

func (h *ModelHandlers) AdminDeleteAllModelsHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteAllModels(w, r)
}

// AdminRegistryHandler handles GET /admin/api/registry
func (h *ModelHandlers) AdminRegistryHandler(w http.ResponseWriter, r *http.Request) {
	reg := h.admin.GetRegistry()
	view := adminRegistryView{
		Catalogue:  reg.Catalogue,
		Providers:  getProvidersView(h.admin),
		MCPServers: reg.MCPServers,
	}
	respondJSON(w, view)
}

// AdminRegistryPutHandler handles PUT /admin/api/registry
func (h *ModelHandlers) AdminRegistryPutHandler(w http.ResponseWriter, r *http.Request) {
	var req adminRegistryView
	if !decodeJSONBody(w, r, &req) {
		return
	}

	err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		reg.Catalogue = req.Catalogue
		reg.Providers = translateProvidersToRegistry(req.Providers)
		reg.MCPServers = req.MCPServers
		// Clear any primary/fallback that pointed at a now-removed model.
		models.ClearDanglingModelRefs(reg)
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

// AdminListProviderManifestsHandler handles GET /admin/api/providers/manifests
func (h *ModelHandlers) AdminListProviderManifestsHandler(w http.ResponseWriter, r *http.Request) {
	manifests := providers.GetRegistry().List()
	respondJSON(w, manifests)
}

// AdminListProviderModelsHandler handles GET /admin/api/providers/models
func (h *ModelHandlers) AdminListProviderModelsHandler(w http.ResponseWriter, r *http.Request) {
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

// AdminTestProviderConnectionHandler handles GET /admin/api/providers/test
func (h *ModelHandlers) AdminTestProviderConnectionHandler(w http.ResponseWriter, r *http.Request) {
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
	Name             string                `json:"name"`
	Provider         string                `json:"provider"`
	Filename         string                `json:"filename"`
	ModelID          string                `json:"model_id"`
	Path             string                `json:"path"`
	Args             []string              `json:"args"`
	Port             int                   `json:"port"`
	ProviderConfig   models.ProviderConfig `json:"provider_config"`
	Metadata         *models.ModelMetadata `json:"metadata"`
	Prefill          *bool                 `json:"prefill,omitempty"`
	ReasoningEnabled *bool                 `json:"reasoning_enabled,omitempty"`
	MaxSteps         int                   `json:"max_steps"`
	ContextBudget    int                   `json:"context_budget"`
	MaxTokens        int                   `json:"max_tokens"`
	Temperature      float64               `json:"temperature"`
	ReasoningBudget  int                   `json:"reasoning_budget"`
	SlotTimeout      int                   `json:"slot_timeout"`
	TimeoutMinutes   int                   `json:"timeout_minutes"`
	ToolCallFormat   string                `json:"tool_call_format"`
	Pricing          *models.ModelPricing  `json:"pricing"`
	Limits           *models.ModelLimits   `json:"limits"`
	Meta             *models.ModelMeta     `json:"meta"`

	// Published capabilities from the provider's live catalog (Phase 2):
	// forwarded from ListModels so the cloud policy can clamp to what the
	// model actually publishes (min(published, tier)).
	ContextLength   int `json:"context_length,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`

	// Published capabilities carried into the runtime ModelConfig (computed,
	// non-persistent).  Set by enrichMetadataFromProviders from the catalog
	// fields above.
	PublishedOutputCap     int `json:"-"`
	PublishedContextLength int `json:"-"`

	ToolTimeoutSeconds           int    `json:"tool_timeout_seconds"`
	FilesystemToolTimeoutSeconds int    `json:"filesystem_tool_timeout_seconds"`
	MaxPlanDurationMinutes       int    `json:"max_plan_duration_minutes"`
	MaxPlanSteps                 int    `json:"max_plan_steps"`
	GuardrailTimeoutSeconds      int    `json:"guardrail_timeout_seconds"`
	GuardrailTimeoutBehavior     string `json:"guardrail_timeout_behavior"`
}

// workloadClass classifies the submitted form. The classify func is the
// runtime's hydrated classifier (per-credential base_url overrides + provider
// defaults applied, single source of truth with the runtime boundary); when
// nil it falls back to the pure classifier over the submitted fields (provider
// label, GGUF artifact, submitted base URL). The runtime re-classifies with the
// full hydrated endpoint on save regardless.
func (r *modelFormRequest) workloadClass(classify func(models.ModelConfig) models.WorkloadClass) models.WorkloadClass {
	cfg := models.ModelConfig{
		Name:     r.Name,
		Provider: r.Provider,
		Filename: r.Filename,
		Path:     r.Path,
		ProviderConfig: &models.ProviderConfig{
			APIKey:     r.ProviderConfig.APIKey,
			APIKeyName: r.ProviderConfig.APIKeyName,
			BaseURL:    r.ProviderConfig.BaseURL,
		},
	}
	if classify != nil {
		return classify(cfg)
	}
	return models.NewWorkloadClassifier("", nil).Classify(cfg)
}

// enrichMetadataFromProviders populates model metadata from the provider's
// model-listing API (meta, limits) and auto-computes InternalCreditWeight
// from OpenRouter pricing.  When exact context length is available, it
// zeroes the provider-tier defaults so ApplyMetadataDefaults can derive
// correct max_tokens/context_budget from the model's actual capabilities.
//
// Workload-aware (H1): local workloads preserve their serving n_ctx, skip
// pricing-derived ICU tuning, and never receive cloud zeroing behaviour.  The
// classify func is the runtime's hydrated classifier; when nil the pure
// classifier (provider label, GGUF artifact, submitted base URL) is used.  The
// backend re-classifies with the full hydrated endpoint on save.
func (r *modelFormRequest) enrichMetadataFromProviders(classify func(models.ModelConfig) models.WorkloadClass) {
	logging.Debug("[enrich] input",
		"meta", r.Meta, "limits", r.Limits, "pricing", r.Pricing,
		"max_tokens", r.MaxTokens, "ctx_budget", r.ContextBudget)

	workload := r.workloadClass(classify)

	if workload != models.WorkloadLocal && r.Pricing != nil && r.ProviderConfig.InternalCreditWeight <= 0 {
		r.ProviderConfig.InternalCreditWeight = orchestrator.ComputeICUWeightFromPricing(r.Pricing)
	}
	if r.Metadata == nil && r.Limits != nil && r.Limits.Context > 0 {
		r.Metadata = &models.ModelMetadata{ContextLength: r.Limits.Context}
		logging.Debug("[enrich] metadata from limits", "context", r.Limits.Context)
	}

	// Resolve the serving context via the orchestrator's single source of
	// truth (B5).  This prefers n_ctx (serving context), then n_ctx_train
	// capped at defaultLocalContextMax (1_048_576), then the 8192 local
	// default — replacing the handler's bespoke walk that silently dropped
	// any training context above 128K.  The handler can never leak a cloud
	// guess here because the resolver is workload-scoped to local serving
	// metadata.
	metaCtx := 0
	if r.Meta != nil {
		metaCtx = orchestrator.ResolveLocalContext(&models.ModelConfig{
			Metadata: &models.ModelMetadata{
				Nctx:          r.Meta.Nctx,
				ContextLength: r.Meta.ContextLength,
				Parameters:    r.Meta.Parameters,
			},
		})
		logging.Debug("[enrich] resolved serving context", "ctx", metaCtx)
	} else if r.Metadata != nil {
		metaCtx = orchestrator.ResolveLocalContext(&models.ModelConfig{Metadata: r.Metadata})
	}

	if r.Metadata == nil && metaCtx > 0 {
		r.Metadata = &models.ModelMetadata{ContextLength: r.Meta.ContextLength, Nctx: metaCtx, Parameters: r.Meta.Parameters}
	}
	if r.Metadata != nil && metaCtx > 0 {
		if r.Meta != nil {
			r.Metadata.Parameters = r.Meta.Parameters
		}
		r.Metadata.Nctx = metaCtx
	}

	// Published capabilities (Phase 2): keep the live-catalog numbers for the
	// cloud policy chain to clamp against.  Local workloads ignore them — the
	// local policy never reads published output caps.
	if r.MaxOutputTokens > 0 {
		r.PublishedOutputCap = r.MaxOutputTokens
	}
	if r.ContextLength > 0 && r.PublishedContextLength == 0 {
		r.PublishedContextLength = r.ContextLength
	}

	// Local workloads always preserve serving metadata — never zero it into a
	// cloud calculation.  Derived budget fields submitted by the UI are
	// cleared so the n_ctx math wins (H1: local workloads ignore submitted
	// budget fields; the backend remains authoritative).
	if workload == models.WorkloadLocal {
		logging.Debug("[enrich] local workload: preserving serving metadata", "ctx", r.Metadata)
		r.MaxTokens = 0
		r.ContextBudget = 0
		r.ReasoningBudget = 0
		return
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

// resolvePublishedCapabilitiesFromCatalog fills the cloud model's published
// context length and output cap from the provider's LIVE catalog when the
// submitted form did not carry them (V5 / Phase 2).  The backend is
// authoritative: a cloud model's real numbers come from the catalog, not from
// client-preferred defaults.  Local workloads are never touched (their serving
// context comes from /slots or GGUF metadata, not a cloud catalog).  Failures
// are silent — the tier row remains the fallback.
//
// The caps are ALSO carried into req.Metadata (persisted) so the cloud clamp
// survives a restart — ModelConfig.Published* fields are json:"-", so the
// persisted carrier is ModelMetadata (Phase 2 "carry MaxOutputTokens into
// ModelMetadata").  Setting Metadata.ContextLength also makes
// enrichMetadataFromProviders take the cloud recomputation path (zeroing the
// prefilled tier defaults), which is what lets ApplyMetadataDefaults clamp to
// the published cap.
func resolvePublishedCapabilitiesFromCatalog(ctx context.Context, runtime RuntimeService, req *modelFormRequest) {
	// Gate on the workload class (hydrated classifier over provider label, GGUF
	// artifact, and the effective endpoint host) — a local-URL openai model
	// (including a per-credential loopback base_url) must never consult a cloud
	// catalog.
	workload := req.workloadClass(runtime.ClassifyModel)
	if workload == models.WorkloadLocal {
		return
	}
	if req.PublishedOutputCap > 0 && req.PublishedContextLength > 0 {
		return
	}
	modelID := req.ModelID
	if modelID == "" {
		modelID = req.Name
	}
	if modelID == "" {
		return
	}
	infos, err := runtime.ListProviderModels(ctx, req.Provider, req.ProviderConfig.APIKeyName)
	if err != nil {
		logging.Debug("[enrich] catalog lookup skipped", "provider", req.Provider, "model", modelID, "err", err)
		return
	}
	for _, info := range infos {
		if info.ID != modelID {
			continue
		}
		if req.PublishedOutputCap == 0 && info.MaxOutputTokens > 0 {
			req.PublishedOutputCap = info.MaxOutputTokens
		}
		if req.PublishedContextLength == 0 && info.ContextLength > 0 {
			req.PublishedContextLength = info.ContextLength
		}
		// Persist the published caps on the model so the clamp survives a
		// restart.  Create Metadata when the catalog is the only source (new
		// ListModels no longer forwards Meta/Limits).
		if info.MaxOutputTokens > 0 || info.ContextLength > 0 {
			if req.Metadata == nil {
				req.Metadata = &models.ModelMetadata{}
			}
			if req.Metadata.MaxOutputTokens == 0 && info.MaxOutputTokens > 0 {
				req.Metadata.MaxOutputTokens = info.MaxOutputTokens
			}
			if req.Metadata.ContextLength == 0 && info.ContextLength > 0 {
				req.Metadata.ContextLength = info.ContextLength
			}
		}
		return
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
// Budget fields (Phase 3): explicit cloud values that differ from the
// policy-derived baseline ARE persisted so deliberate UI edits stick across
// Sync().  Local workloads never persist budget fields — their values are
// always n_ctx-derived — and any stale persisted budget override is removed.

// modelBudgetOverride carries the explicit submitted budget values and the
// derived baseline, so writeModelOverrides can decide what (if anything) to
// persist for max_tokens / context_budget.
type modelBudgetOverride struct {
	// Explicit values as submitted by the UI form (0 = not submitted).
	ExplicitMaxTokens int
	ExplicitCtxBudget int
	// Derived values the policy computed (the baseline).
	DerivedMaxTokens int
	DerivedCtxBudget int
	// WorkloadLocal models never persist budget fields.
	WorkloadClass models.WorkloadClass
}

// hasModelOverrides returns true when the model config contains at least one
// non-zero agent-tuning field that should be persisted as a user override.
// MaxTokens/ContextBudget are decided separately via modelBudgetOverride
// (cloud explicit values differ from baseline persist; local never).
func hasModelOverrides(cfg models.ModelConfig) bool {
	return cfg.MaxSteps > 0 || cfg.ReasoningBudget > 0 || cfg.ReasoningEnabled != nil || cfg.SlotTimeout > 0 ||
		cfg.ToolCallFormat != "" || (cfg.Prefill != nil && *cfg.Prefill) || cfg.TimeoutMinutes > 0 ||
		cfg.Temperature > 0 ||
		cfg.ToolTimeoutSeconds > 0 || cfg.FilesystemToolTimeoutSeconds > 0 ||
		cfg.MaxPlanDurationMinutes > 0 || cfg.MaxPlanSteps > 0 ||
		cfg.GuardrailTimeoutSeconds > 0 || cfg.GuardrailTimeoutBehavior != "" ||
		(cfg.ProviderConfig != nil && cfg.ProviderConfig.InternalCreditWeight > 0)
}

// budgetOverridesToPersist returns the explicit cloud max_tokens/context_budget
// values that differ from the derived baseline (deliberate user edits), or
// zero values when nothing should persist.  Local workloads always return
// zero — their budget is n_ctx-derived and must not be frozen.
func (b modelBudgetOverride) budgetOverridesToPersist() (maxTokens, ctxBudget int) {
	if b.WorkloadClass == models.WorkloadLocal {
		return 0, 0
	}
	if b.ExplicitMaxTokens != 0 && b.ExplicitMaxTokens != b.DerivedMaxTokens {
		maxTokens = b.ExplicitMaxTokens
	}
	if b.ExplicitCtxBudget != 0 && b.ExplicitCtxBudget != b.DerivedCtxBudget {
		ctxBudget = b.ExplicitCtxBudget
	}
	return maxTokens, ctxBudget
}

// writeModelOverrides persists agent-tuning fields to settings.yml
// (model_overrides).  When the model carries no overrides, any stale persisted
// entry for the model is REMOVED so "unset" actually restores the provider
// default (hadOverride reports whether the model had a persisted entry before
// this save, so a settings write is skipped when there is nothing to change).
func writeModelOverrides(name string, cfg models.ModelConfig, budget modelBudgetOverride, hadOverride bool, updateFn func(func(*models.UserSettings)) error) {
	persistMax, persistCtx := budget.budgetOverridesToPersist()
	has := hasModelOverrides(cfg) || persistMax != 0 || persistCtx != 0
	if !has && !hadOverride {
		return
	}
	_ = updateFn(func(s *models.UserSettings) {
		if !has {
			delete(s.ModelOverrides, name)
			return
		}
		if s.ModelOverrides == nil {
			s.ModelOverrides = make(map[string]models.ModelOverride)
		}
		weight := float64(0)
		if cfg.ProviderConfig != nil {
			weight = cfg.ProviderConfig.InternalCreditWeight
		}
		s.ModelOverrides[name] = models.ModelOverride{
			MaxSteps:                     cfg.MaxSteps,
			ContextBudget:                persistCtx,
			MaxTokens:                    persistMax,
			ReasoningBudget:              cfg.ReasoningBudget,
			SlotTimeout:                  cfg.SlotTimeout,
			Temperature:                  cfg.Temperature,
			ICUWeight:                    weight,
			ToolCallFormat:               cfg.ToolCallFormat,
			Prefill:                      cfg.Prefill,
			ReasoningEnabled:             cfg.ReasoningEnabled,
			TimeoutMinutes:               cfg.TimeoutMinutes,
			ToolTimeoutSeconds:           cfg.ToolTimeoutSeconds,
			FilesystemToolTimeoutSeconds: cfg.FilesystemToolTimeoutSeconds,
			MaxPlanDurationMinutes:       cfg.MaxPlanDurationMinutes,
			MaxPlanSteps:                 cfg.MaxPlanSteps,
			GuardrailTimeoutSeconds:      cfg.GuardrailTimeoutSeconds,
			GuardrailTimeoutBehavior:     cfg.GuardrailTimeoutBehavior,
		}
	})
}

// modelConfigFromRequest builds a ModelConfig from a form request, capturing
// the agent-tuning fields and identity fields in one place.  The caller
// provides the varying parameters (filename, path, args, environment) rather
// than repeating the full struct literal at every call site.
func modelConfigFromRequest(req modelFormRequest, filename, path string, args []string, env map[string]string) models.ModelConfig {
	return models.ModelConfig{
		Name:             req.Name,
		Provider:         req.Provider,
		Filename:         filename,
		Path:             path,
		Args:             args,
		Port:             req.Port,
		Environment:      env,
		ProviderConfig:   &req.ProviderConfig,
		Metadata:         req.Metadata,
		Prefill:          req.Prefill,
		ReasoningEnabled: req.ReasoningEnabled,
		MaxSteps:         req.MaxSteps,
		ContextBudget:    req.ContextBudget,
		MaxTokens:        req.MaxTokens,
		Temperature:      req.Temperature,
		ReasoningBudget:  req.ReasoningBudget,
		SlotTimeout:      req.SlotTimeout,
		TimeoutMinutes:   req.TimeoutMinutes,
		ToolCallFormat:   req.ToolCallFormat,

		PublishedOutputCap:     req.PublishedOutputCap,
		PublishedContextLength: req.PublishedContextLength,

		ToolTimeoutSeconds:           req.ToolTimeoutSeconds,
		FilesystemToolTimeoutSeconds: req.FilesystemToolTimeoutSeconds,
		MaxPlanDurationMinutes:       req.MaxPlanDurationMinutes,
		MaxPlanSteps:                 req.MaxPlanSteps,
		GuardrailTimeoutSeconds:      req.GuardrailTimeoutSeconds,
		GuardrailTimeoutBehavior:     req.GuardrailTimeoutBehavior,
	}
}

// registerModelFromRequest runs the model-registration pipeline: catalog
// capability enrichment, metadata defaults, and runtime add. It returns the
// runtime config, used by handleAddModel.
//
// req is mutated in place by the enrichment steps; callers must use the same
// req when persisting (persistModelAndOverrides) so the persisted Metadata
// carries the catalog-derived capabilities.
func registerModelFromRequest(
	ctx context.Context,
	runtime RuntimeService,
	admin AdminService,
	req *modelFormRequest,
	filename, fullPath string,
	runtimeArgs []string,
) (models.ModelConfig, error) {
	resolvePublishedCapabilitiesFromCatalog(ctx, runtime, req)
	req.enrichMetadataFromProviders(runtime.ClassifyModel)

	logging.Debug("[registerModel] before ApplyMetadataDefaults", "name", req.Name, "ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens, "reasoning", req.ReasoningBudget, "metadata", req.Metadata)
	runtimeCfg := modelConfigFromRequest(*req, filename, fullPath, runtimeArgs, admin.Environment())
	orchestrator.ApplyMetadataDefaults(&runtimeCfg)
	logging.Info("[registerModel] after ApplyMetadataDefaults", "name", runtimeCfg.Name, "ctx_budget", runtimeCfg.ContextBudget, "max_tokens", runtimeCfg.MaxTokens, "reasoning", runtimeCfg.ReasoningBudget)

	if err := runtime.AddModel(runtimeCfg); err != nil {
		return runtimeCfg, err
	}
	return runtimeCfg, nil
}

// persistModelAndOverrides builds the persistence copy of a model config,
// applies metadata defaults, persists it via persistFn, then writes the
// agent-tuning overrides (Phase 3 budget persistence included). It is the
// non-HTTP core used by persistAndWriteOverrides.
func persistModelAndOverrides(
	admin AdminService,
	req modelFormRequest,
	filename string,
	persistFn func(models.ModelConfig) error,
	runtimeCfg models.ModelConfig,
	submittedMaxTokens, submittedCtxBudget int,
) error {
	persistCfg := modelConfigFromRequest(req, filename, "", append([]string{}, req.Args...), nil)
	orchestrator.ApplyMetadataDefaults(&persistCfg)
	if err := persistFn(persistCfg); err != nil {
		return err
	}

	// Whether the model had a persisted override before this save — used by
	// writeModelOverrides to clear a stale entry when no override remains.
	_, hadOverride := admin.GetSettings().ModelOverrides[req.Name]

	budgetOverride := modelBudgetOverride{
		ExplicitMaxTokens: submittedMaxTokens,
		ExplicitCtxBudget: submittedCtxBudget,
		DerivedMaxTokens:  persistCfg.MaxTokens,
		DerivedCtxBudget:  persistCfg.ContextBudget,
		WorkloadClass:     persistCfg.WorkloadClass,
	}
	writeModelOverrides(req.Name, runtimeCfg, budgetOverride, hadOverride, admin.UpdateSettings)
	return nil
}

// persistAndWriteOverrides is the HTTP wrapper around persistModelAndOverrides:
// it persists, writes overrides, and responds with the runtime config. Shared
// by handleAddModel/handleUpdateModel — the two call sites differ only in the
// persist target and error phrasing.
func (h *ModelHandlers) persistAndWriteOverrides(
	w http.ResponseWriter,
	req modelFormRequest,
	filename string,
	persistFn func(models.ModelConfig) error,
	persistErrMsg string,
	runtimeCfg models.ModelConfig,
	submittedMaxTokens, submittedCtxBudget int,
) {
	if err := persistModelAndOverrides(h.admin, req, filename, persistFn, runtimeCfg, submittedMaxTokens, submittedCtxBudget); err != nil {
		writeJSONError(w, http.StatusInternalServerError, persistErrMsg+err.Error())
		return
	}
	respondJSON(w, runtimeCfg)
}

// handleAddModel registers a new model in the runtime and persists it.
// For OpenRouter (and similar providers that return pricing), it
// auto-computes InternalCreditWeight from the pricing data so ICU weight
// is accurate without manual configuration.
func (h *ModelHandlers) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req modelFormRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	logging.Info("[addModel] received", "name", req.Name, "provider", req.Provider,
		"meta", req.Meta, "limits", req.Limits, "pricing", req.Pricing,
		"max_steps", req.MaxSteps, "ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens,
		"temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)

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

	// Capture the submitted budget values BEFORE enrichment zeroes derived
	// fields, so explicit cloud edits can be distinguished from the derived
	// baseline (Phase 3).
	submittedMaxTokens := req.MaxTokens
	submittedCtxBudget := req.ContextBudget

	// Register the model through the shared pipeline (registerModelFromRequest),
	// which pulls the published context window / output cap from the live
	// catalog (V5) and enriches metadata before applying defaults and adding to
	// the runtime.
	runtimeCfg, err := registerModelFromRequest(r.Context(), h.runtime, h.admin, &req, filename, fullPath, runtimeArgs)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrModelExists) {
			status = http.StatusConflict
		}
		writeJSONError(w, status, "unable to add model: "+err.Error())
		return
	}

	h.persistAndWriteOverrides(w, req, filename, h.admin.PersistModel, "saved model but failed to persist config: ", runtimeCfg, submittedMaxTokens, submittedCtxBudget)
}

// handleUpdateModel updates an existing model in both runtime and
// registry persistence.  Like handleAddModel, it auto-computes ICU weight
// from pricing data when available.
func (h *ModelHandlers) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req modelFormRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	logging.Info("[updateModel] body decoded", "name", req.Name, "provider", req.Provider,
		"meta", req.Meta, "limits", req.Limits, "pricing", req.Pricing,
		"ctx_budget", req.ContextBudget, "max_tokens", req.MaxTokens,
		"temperature", req.Temperature, "reasoning", req.ReasoningBudget)

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

	// Capture submitted budget values before enrichment zeroes derived fields
	// (Phase 3 — explicit cloud edits persist, local never does).
	submittedMaxTokens := req.MaxTokens
	submittedCtxBudget := req.ContextBudget

	// Backend-authoritative catalog lookup (V5): same as add — published caps
	// from the live catalog keep the cloud clamp accurate on edits too.  Runs
	// before enrichment so the persisted Metadata drives cloud recomputation.
	resolvePublishedCapabilitiesFromCatalog(r.Context(), h.runtime, &req)

	req.enrichMetadataFromProviders(h.runtime.ClassifyModel)

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(req.Filename, req.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeJSONError(w, http.StatusBadRequest, "model file not found")
			return
		}
	}

	runtimeCfg := modelConfigFromRequest(req, req.Filename, fullPath, runtimeArgs, h.admin.Environment())
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

	h.persistAndWriteOverrides(w, req, req.Filename, h.admin.PersistReplaceModel, "updated model but failed to persist config: ", runtimeCfg, submittedMaxTokens, submittedCtxBudget)
}

func (h *ModelHandlers) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
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

func (h *ModelHandlers) handleDeleteAllModels(w http.ResponseWriter, r *http.Request) {
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
