// cloud_policy_test.go — CloudBudgetPolicy behaviour tests (§5 cloud rows):
// tier fallback, published-cap clamping, small-context clamp-and-succeed (V3),
// and the typed ErrCapabilityImpossible.
package orchestrator

import (
	"errors"
	"testing"

	"llm-proxy/models"
)

func TestCloudPolicy_NvidiaNoMetadata(t *testing.T) {
	cfg := models.ModelConfig{Provider: "nvidia", Name: "laguna-xs-2.1"}
	ctx := ContextResolution{PublishedContext: 128_000, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 8192 {
		t.Fatalf("expected max_tokens 8192 (tier row), got %d", b.MaxTokens)
	}
	if b.ContextBudget != 20000 {
		t.Fatalf("expected context_budget 20000 (nvidia tier history), got %d", b.ContextBudget)
	}
}

func TestCloudPolicy_GeminiNoMetadata(t *testing.T) {
	cfg := models.ModelConfig{Provider: "gemini", Name: "gemini-2"}
	ctx := ContextResolution{PublishedContext: 1_048_576, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 8192 {
		t.Fatalf("expected max_tokens 8192 (tier row), got %d", b.MaxTokens)
	}
	if b.ContextBudget != 50000 {
		t.Fatalf("expected context_budget 50000 (gemini tier history), got %d", b.ContextBudget)
	}
}

func TestCloudPolicy_OpenRouterPublishedCap(t *testing.T) {
	// Published cap 16384 > tier 8192 → clamps to 8192.
	cfg := models.ModelConfig{Provider: "openrouter", Name: "model", PublishedOutputCap: 16384}
	ctx := ContextResolution{PublishedContext: 131_072, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 8192 {
		t.Fatalf("expected max_tokens 8192 (min(published 16384, tier 8192)), got %d", b.MaxTokens)
	}
}

func TestCloudPolicy_OpenRouterCapBelowTier(t *testing.T) {
	// Published cap 4096 < tier 8192 → clamp to published cap.
	cfg := models.ModelConfig{Provider: "openrouter", Name: "model", PublishedOutputCap: 4096}
	ctx := ContextResolution{PublishedContext: 131_072, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 4096 {
		t.Fatalf("expected max_tokens 4096 (clamped to published cap), got %d", b.MaxTokens)
	}
}

// TestCloudPolicy_SmallContextCapless_Succeeds is V3: a 4K-context capless
// model clamps to max(1, ctx − reserve) and succeeds — never ErrCapabilityImpossible.
func TestCloudPolicy_SmallContextCapless_Succeeds(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openrouter", Name: "tiny"}
	ctx := ContextResolution{PublishedContext: 4096, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("4K capless model must clamp and succeed, got %v", err)
	}
	// reserve = max(tier cap 8192, promptReserveGuess) → 8192; clamps to 1.
	want := 1
	if b.MaxTokens != want {
		t.Fatalf("expected max_tokens %d (clamped to max(1, 4096-8192)), got %d", want, b.MaxTokens)
	}
}

// TestCloudPolicy_ContradictoryContext is V3: publishedCtx ≤ minViablySmallContext
// → typed ErrCapabilityImpossible.
func TestCloudPolicy_ContradictoryContext(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openrouter", Name: "tiny"}
	ctx := ContextResolution{PublishedContext: minViablySmallContext, OutputCap: ResolveOutputCap(cfg)}
	_, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err == nil {
		t.Fatal("expected ErrCapabilityImpossible for minViablySmallContext")
	}
	if !errors.Is(err, ErrCapabilityImpossible) {
		t.Fatalf("expected ErrCapabilityImpossible, got %v", err)
	}
}

// TestCloudPolicy_4KContextConstrainsHistory verifies §5: a published 4K
// context constrains the history budget below provider capacity.
func TestCloudPolicy_4KContextConstrainsHistory(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openrouter", Name: "tiny"}
	ctx := ContextResolution{PublishedContext: 4096, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.ContextBudget >= 30000 {
		t.Fatalf("history budget must be constrained below provider capacity (30000), got %d", b.ContextBudget)
	}
}

// TestCloudPolicy_Capless_NoPublishedContext verifies the capless branch:
// nothing published, nothing known → tier history budget, tier output cap.
func TestCloudPolicy_Capless_NoPublishedContext(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openrouter", Name: "model"}
	ctx := ContextResolution{PublishedContext: 0, OutputCap: ResolveOutputCap(cfg)}
	b, err := (CloudBudgetPolicy{}).Derive(cfg, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 8192 {
		t.Fatalf("expected max_tokens 8192 (tier), got %d", b.MaxTokens)
	}
	if b.ContextBudget != 30000 {
		t.Fatalf("expected context_budget 30000 (tier history), got %d", b.ContextBudget)
	}
}

func TestResolveOutputCap_PublishedBeatsTier(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openrouter", Name: "model", PublishedOutputCap: 4096}
	if got := ResolveOutputCap(cfg); got != 4096 {
		t.Fatalf("expected published cap 4096 to win, got %d", got)
	}
	cfg2 := models.ModelConfig{Provider: "openrouter", Name: "model"}
	if got := ResolveOutputCap(cfg2); got != 8192 {
		t.Fatalf("expected tier cap 8192 fallback, got %d", got)
	}
}

// TestApplyMetadataDefaults_CloudOverridePreserved verifies explicit cloud
// max_tokens/context_budget are not overwritten by ApplyMetadataDefaults.
func TestApplyMetadataDefaults_CloudOverridePreserved(t *testing.T) {
	cfg := &models.ModelConfig{
		Name:           "test-model",
		Provider:       "openrouter",
		MaxTokens:      2048,
		ContextBudget:  8192,
		WorkloadClass:  models.WorkloadCloud,
	}
	ApplyMetadataDefaults(cfg)
	if cfg.MaxTokens != 2048 {
		t.Fatalf("explicit max_tokens must be preserved, got %d", cfg.MaxTokens)
	}
	if cfg.ContextBudget != 8192 {
		t.Fatalf("explicit context_budget must be preserved, got %d", cfg.ContextBudget)
	}
}

// TestApplyMetadataDefaults_RestartDurability verifies the published output cap
// carried on the PERSISTED ModelMetadata (Phase 2 carrier) clamps a cloud model
// after a restart — ModelConfig.PublishedOutputCap is json:"-", so the clamp
// must survive via Metadata.MaxOutputTokens.  A model with MaxTokens zeroed (as
// Sync does) and a published cap below the tier row derives the clamped value,
// not the tier default.
func TestApplyMetadataDefaults_RestartDurability(t *testing.T) {
	// 4096 published cap < openrouter tier 8192.  After a restart Sync zeroes
	// MaxTokens/ContextBudget and re-derives from the persisted metadata.
	cfg := &models.ModelConfig{
		Name:          "gpt-4o",
		Provider:      "openrouter",
		Metadata:      &models.ModelMetadata{ContextLength: 128_000, MaxOutputTokens: 4096},
		WorkloadClass: models.WorkloadCloud,
	}
	ApplyMetadataDefaults(cfg)
	if cfg.MaxTokens != 4096 {
		t.Fatalf("expected max_tokens clamped to published cap 4096 after restart, got %d", cfg.MaxTokens)
	}
	if cfg.ContextBudget == 0 {
		t.Fatal("expected a derived context_budget after restart")
	}
}

// TestApplyMetadataDefaults_PrefillClampedToPublishedCap verifies the
// frontend-prefill scenario: a cloud form prefilled with the tier default 8192
// max_tokens is clamped down to the published cap when the metadata-driven
// recomputation path zeroes the prefill (enrichMetadataFromProviders) and
// ApplyMetadataDefaults re-derives from the published cap.
func TestApplyMetadataDefaults_PrefillClampedToPublishedCap(t *testing.T) {
	// Simulate post-enrichment state: enrichment zeroed the prefilled tier
	// defaults because Metadata.ContextLength is set from the catalog, so
	// ApplyMetadataDefaults derives from the published cap (4096 < tier 8192).
	cfg := &models.ModelConfig{
		Name:          "gpt-4o",
		Provider:      "openrouter",
		Metadata:      &models.ModelMetadata{ContextLength: 128_000, MaxOutputTokens: 4096},
		WorkloadClass: models.WorkloadCloud,
	}
	ApplyMetadataDefaults(cfg)
	if cfg.MaxTokens != 4096 {
		t.Fatalf("expected max_tokens 4096 (clamped to published cap), got %d", cfg.MaxTokens)
	}
}

// TestResolveOutputCap_MetadataFallback verifies the restart path: the output
// cap chain reads the persisted ModelMetadata.MaxOutputTokens when the runtime
// PublishedOutputCap field is 0 (json:"-", lost on reload).
func TestResolveOutputCap_MetadataFallback(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "openrouter",
		Metadata: &models.ModelMetadata{MaxOutputTokens: 4096},
	}
	if got := ResolveOutputCap(cfg); got != 4096 {
		t.Fatalf("expected published cap 4096 from persisted metadata, got %d", got)
	}
	// Tier fallback when neither runtime field nor metadata carries a cap.
	cfg2 := models.ModelConfig{Provider: "openrouter"}
	if got := ResolveOutputCap(cfg2); got != 8192 {
		t.Fatalf("expected tier cap 8192 fallback, got %d", got)
	}
}
