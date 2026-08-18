package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

// TestEnrichLocalIgnoresBudgetFields verifies H1: local workloads (including
// openai-slugged models pointed at a local URL) clear submitted derived budget
// fields so the n_ctx math wins, and never receive pricing-derived ICU tuning.
func TestEnrichLocalIgnoresBudgetFields(t *testing.T) {
	tests := []struct {
		name           string
		req            modelFormRequest
		wantLocalClear bool
	}{
		{
			name: "openai + gguf filename",
			req: modelFormRequest{
				Provider:      "openai",
				Name:          "qwen3.5-9b",
				Filename:      "Qwen3.5-9B-UD-Q4_K_XL.gguf",
				MaxTokens:     9999,
				ContextBudget: 99999,
				Pricing:       &models.ModelPricing{Prompt: "0.0000025", Completion: "0.00001"},
			},
			wantLocalClear: true,
		},
		{
			name: "openai + local base url",
			req: modelFormRequest{
				Provider:      "openai",
				Name:          "llama-alias",
				MaxTokens:     9999,
				ContextBudget: 99999,
				ProviderConfig: models.ProviderConfig{BaseURL: "http://127.0.0.1:8080"},
				Pricing:       &models.ModelPricing{Prompt: "0.0000025", Completion: "0.00001"},
			},
			wantLocalClear: true,
		},
		{
			name: "openai cloud keeps pricing weight",
			req: modelFormRequest{
				Provider:      "openai",
				Name:          "gpt-4o",
				MaxTokens:     4096,
				ContextBudget: 50000,
				ProviderConfig: models.ProviderConfig{BaseURL: "https://api.openai.com/v1"},
				Pricing:       &models.ModelPricing{Prompt: "0.0000025", Completion: "0.00001"},
			},
			wantLocalClear: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			req.enrichMetadataFromProviders(nil)
			if tt.wantLocalClear {
				if req.MaxTokens != 0 || req.ContextBudget != 0 || req.ReasoningBudget != 0 {
					t.Fatalf("local workload must clear submitted budget fields, got max_tokens=%d ctx_budget=%d", req.MaxTokens, req.ContextBudget)
				}
				if req.ProviderConfig.InternalCreditWeight != 0 {
					t.Fatalf("local workload must not receive pricing-derived ICU weight, got %f", req.ProviderConfig.InternalCreditWeight)
				}
		} else {
			if req.ProviderConfig.InternalCreditWeight == 0 {
				t.Fatal("cloud workload should receive pricing-derived ICU weight")
			}
		}
	})
	}
}

// TestEnrichHydratedCredentialLoopback verifies the registration edge uses the
// runtime's hydrated classifier: an openai slug whose per-credential base_url is
// a loopback endpoint is treated as a local workload, so enrichment never grants
// cloud behaviour (pricing weight, kept budget fields). The api_key_name must be
// forwarded into the classification so the runtime can hydrate the credential.
func TestEnrichHydratedCredentialLoopback(t *testing.T) {
	req := &modelFormRequest{
		Provider:    "openai",
		Name:        "llama-alias",
		ModelID:     "llama-alias",
		MaxTokens:   9999,
		ContextBudget: 99999,
		ProviderConfig: models.ProviderConfig{APIKeyName: "loopback-key"},
		Pricing:     &models.ModelPricing{Prompt: "0.0000025", Completion: "0.00001"},
	}
	classify := func(cfg models.ModelConfig) models.WorkloadClass {
		if cfg.ProviderConfig == nil || cfg.ProviderConfig.APIKeyName != "loopback-key" {
			t.Fatalf("expected api_key_name forwarded for hydration, got %+v", cfg.ProviderConfig)
		}
		return models.WorkloadLocal
	}
	req.enrichMetadataFromProviders(classify)
	if req.MaxTokens != 0 || req.ContextBudget != 0 || req.ReasoningBudget != 0 {
		t.Fatalf("local workload must clear submitted budget fields, got max_tokens=%d ctx_budget=%d", req.MaxTokens, req.ContextBudget)
	}
	if req.ProviderConfig.InternalCreditWeight != 0 {
		t.Fatalf("local workload must not receive pricing-derived ICU weight, got %f", req.ProviderConfig.InternalCreditWeight)
	}
}

// TestResolvePublishedCapabilitiesFromCatalog verifies V5 / Phase 2: for a cloud
// model the backend fills PublishedOutputCap / PublishedContextLength from the
// provider's live catalog when the form did not carry them, so CloudBudgetPolicy
// clamps to what the model actually publishes.  Local workloads are never
// touched, and catalog failures fall back silently to the tier row.
func TestResolvePublishedCapabilitiesFromCatalog(t *testing.T) {
	infos := []models.ProviderModelInfo{
		{ID: "deepseek/deepseek-v4-flash", MaxOutputTokens: 384000, ContextLength: 1048576},
		{ID: "some/gpt-4o", MaxOutputTokens: 4096, ContextLength: 128000},
	}

	cases := []struct {
		name        string
		provider    string
		modelID     string
		catalogErr  error
		formCaps    bool // form already carries published caps
		wantCap     int
		wantCtx     int
		wantCatalog bool // catalog should have been consulted
		loopbackKey bool // per-credential loopback base_url → hydrated local
	}{
		{
			name:        "cloud fills caps from catalog",
			provider:    "openrouter",
			modelID:     "deepseek/deepseek-v4-flash",
			wantCap:     384000,
			wantCtx:     1048576,
			wantCatalog: true,
		},
		{
			name:        "cloud form caps win",
			provider:    "openrouter",
			modelID:     "deepseek/deepseek-v4-flash",
			formCaps:    true,
			wantCap:     9999,
			wantCtx:     99999,
			wantCatalog: false,
		},
		{
			name:        "cloud no catalog match keeps zeros",
			provider:    "openrouter",
			modelID:     "unknown/model",
			wantCap:     0,
			wantCtx:     0,
			wantCatalog: true,
		},
		{
			name:        "local never consults catalog",
			provider:    "local",
			modelID:     "local-qwen",
			wantCap:     0,
			wantCtx:     0,
			wantCatalog: false,
		},
		{
			name:        "catalog error is silent (tier fallback)",
			provider:    "openrouter",
			modelID:     "deepseek/deepseek-v4-flash",
			catalogErr:  errors.New("catalog unavailable"),
			wantCap:     0,
			wantCtx:     0,
			wantCatalog: true,
		},
		{
			name:        "openai loopback credential never consults catalog",
			provider:    "openai",
			modelID:     "llama-alias",
			wantCap:     0,
			wantCtx:     0,
			wantCatalog: false,
			loopbackKey: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			consulted := false
			runtime := &mocks.MockManager{
				ListProviderModelsFunc: func(ctx context.Context, provider, apiKeyName string) ([]models.ProviderModelInfo, error) {
					consulted = true
					if c.catalogErr != nil {
						return nil, c.catalogErr
					}
					return infos, nil
				},
			}
			if c.loopbackKey {
				runtime.ClassifyModelFunc = func(models.ModelConfig) models.WorkloadClass {
					return models.WorkloadLocal
				}
			}
			h := NewModelHandlers(runtime, &mocks.MockAdminService{})

			req := &modelFormRequest{
				Provider: c.provider,
				Name:     c.modelID,
				ModelID:  c.modelID,
			}
			if c.loopbackKey {
				req.ProviderConfig = models.ProviderConfig{APIKeyName: "loopback-key"}
			}
			if c.formCaps {
				req.PublishedOutputCap = 9999
				req.PublishedContextLength = 99999
			}

			resolvePublishedCapabilitiesFromCatalog(context.Background(), h.runtime, req)

			if req.PublishedOutputCap != c.wantCap || req.PublishedContextLength != c.wantCtx {
				t.Fatalf("published caps = %d/%d, want %d/%d", req.PublishedOutputCap, req.PublishedContextLength, c.wantCap, c.wantCtx)
			}
			if consulted != c.wantCatalog {
				t.Fatalf("catalog consulted = %v, want %v", consulted, c.wantCatalog)
			}
		})
	}
}

// TestEnrichResolvesContextViaOrchestrator verifies B5: the handler delegates
// context resolution to orchestrator.ResolveLocalContext instead of a bespoke
// walk.  The authoritative cap is defaultLocalContextMax (1_048_576), so a
// training context above 128K is no longer silently dropped (the old 128_000
// cap divergence) but capped at 1M and stored as the serving n_ctx.
func TestEnrichResolvesContextViaOrchestrator(t *testing.T) {
	cases := []struct {
		name        string
		meta        *models.ModelMeta
		wantNctx    int
		wantNilMeta bool
	}{
		{
			name:     "n_ctx serving context wins",
			meta:     &models.ModelMeta{Nctx: 8192, ContextLength: 262144},
			wantNctx: 8192,
		},
		{
			name:     "training context under cap used",
			meta:     &models.ModelMeta{ContextLength: 65536},
			wantNctx: 65536,
		},
		{
			name:     "training context above old 128K cap now capped at 1M",
			meta:     &models.ModelMeta{ContextLength: 2000000},
			wantNctx: 1048576,
		},
		{
			name:        "no meta leaves metadata untouched (no serving context known)",
			meta:        nil,
			wantNilMeta: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &modelFormRequest{
				Provider: "local",
				Name:     "test-model",
				Filename: "test.gguf",
				Meta:     c.meta,
			}
			req.enrichMetadataFromProviders(nil)
			if c.wantNilMeta {
				if req.Metadata != nil {
					t.Fatalf("expected nil Metadata, got %+v", req.Metadata)
				}
				return
			}
			if req.Metadata == nil || req.Metadata.Nctx != c.wantNctx {
				t.Fatalf("Metadata.Nctx = %v, want %d (meta=%+v)", req.Metadata, c.wantNctx, c.meta)
			}
		})
	}
}

// TestValidateLoopStrategy verifies the boundary validation: empty and known
// values pass; unknown non-empty values are rejected with a 400 listing the
// registry-derived valid values.
func TestValidateLoopStrategy(t *testing.T) {
	cases := []struct {
		name       string
		value      string
		wantReject bool
	}{
		{"empty passes", "", false},
		{"react passes", "react", false},
		{"plan_execute passes", "plan_execute", false},
		{"unknown rejected", "map_reduce", true},
		{"garbage rejected", "nonsense", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ok := validateLoopStrategy(rr, tc.value)
			if tc.wantReject {
				if ok {
					t.Fatal("expected rejection, got ok")
				}
				if rr.Code != http.StatusBadRequest {
					t.Errorf("expected 400, got %d", rr.Code)
				}
				if !strings.Contains(rr.Body.String(), "react") {
					t.Errorf("expected valid-values hint listing react, got %q", rr.Body.String())
				}
				return
			}
			if !ok {
				t.Fatal("expected acceptance, got rejection")
			}
			if rr.Code != http.StatusOK && rr.Code != 0 {
				t.Errorf("expected no error write, got %d", rr.Code)
			}
		})
	}
}

// TestHasModelOverrides_LoopStrategy verifies a configured loop_strategy counts
// as a persisted override (settings.yml round-trip) like the other tuning fields.
func TestHasModelOverrides_LoopStrategy(t *testing.T) {
	if !hasModelOverrides(models.ModelConfig{LoopStrategy: models.LoopStrategyReact}) {
		t.Error("expected loop_strategy override to count as an override")
	}
	if hasModelOverrides(models.ModelConfig{}) {
		t.Error("expected empty config to have no overrides")
	}
}

