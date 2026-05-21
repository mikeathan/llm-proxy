package orchestrator

import (
	"llm-proxy/models"
	"testing"
)

func TestSqueeze_UnderBudget_NoSqueeze(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       100,
		ReasoningBudget: 50,
		ContextChars:    500,
		ModelContextLen: 8192,
		ICUWeight:       1.0,
		RemainingICU:    10000,
	})
	if !result.Allowed {
		t.Fatal("expected allowed when under budget")
	}
	if result.SqueezeFactor != 1.0 {
		t.Fatalf("expected squeeze 1.0, got %f", result.SqueezeFactor)
	}
	if result.AdjustedMaxTokens != 100 {
		t.Fatalf("expected max_tokens unchanged 100, got %d", result.AdjustedMaxTokens)
	}
}

func TestSqueeze_OverBudget_AppliesSqueeze(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       200,
		ReasoningBudget: 100,
		ContextChars:    1000,
		ModelContextLen: 1000,
		ICUWeight:       1.0,
		RemainingICU:    750,
	})
	if !result.Allowed {
		t.Fatalf("expected allowed, factor=%f adj=%d/%d", result.SqueezeFactor, result.AdjustedMaxTokens, result.AdjustedReasoning)
	}
	if result.SqueezeFactor >= 1.0 {
		t.Fatalf("expected squeeze factor < 1.0, got %f", result.SqueezeFactor)
	}
	if result.AdjustedMaxTokens >= 200 {
		t.Fatalf("expected squeezed max_tokens < 200, got %d", result.AdjustedMaxTokens)
	}
}

func TestSqueeze_HardFloor_NeverBelow(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       100,
		ReasoningBudget: 100,
		ContextChars:    10000,
		ModelContextLen: 1000,
		ICUWeight:       1.0,
		RemainingICU:    10,
	})
	if result.Allowed {
		t.Fatal("expected rejection when cap is tiny")
	}
}

func TestSqueeze_HardFloor_PreventsZero(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       500,
		ReasoningBudget: 0,
		ContextChars:    4000,
		ModelContextLen: 2000,
		ICUWeight:       1.0,
		RemainingICU:    1,
	})
	if result.Allowed {
		t.Fatal("expected rejection at minimal remaining")
	}
	if result.SqueezeFactor < 0.2 {
		t.Fatalf("squeeze factor %f below hard floor 0.2", result.SqueezeFactor)
	}
}

func TestSqueeze_LowDensity_MinimalSqueeze(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       100,
		ReasoningBudget: 0,
		ContextChars:    200,
		ModelContextLen: 10000,
		ICUWeight:       1.0,
		RemainingICU:    200,
	})
	if !result.Allowed {
		t.Fatal("expected allowed for low density")
	}
	if result.SqueezeFactor < 0.95 {
		t.Fatalf("expected minimal squeeze (>=0.95) at low density, got %f", result.SqueezeFactor)
	}
}

func TestSqueeze_ZeroBudget_Allows(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       0,
		ReasoningBudget: 0,
		ContextChars:    0,
		ModelContextLen: 8192,
		ICUWeight:       1.0,
		RemainingICU:    100,
	})
	if !result.Allowed {
		t.Fatal("empty request should be allowed")
	}
}

func TestComputeICUWeightFromPricing_Valid(t *testing.T) {
	pricing := &models.ModelPricing{
		Prompt:     "0.0000025",
		Completion: "0.00001",
	}
	weight := ComputeICUWeightFromPricing(pricing)
	if weight < 12.49 || weight > 12.51 {
		t.Fatalf("expected ~12.5, got %f", weight)
	}
}

func TestComputeICUWeightFromPricing_Nil(t *testing.T) {
	if ComputeICUWeightFromPricing(nil) != 0 {
		t.Fatal("expected 0 for nil pricing")
	}
}

func TestComputeICUWeightFromPricing_Invalid(t *testing.T) {
	pricing := &models.ModelPricing{Prompt: "abc", Completion: "0.001"}
	if ComputeICUWeightFromPricing(pricing) != 0 {
		t.Fatal("expected 0 for invalid pricing")
	}
}

func TestResolveICUWeight_ExplicitConfig(t *testing.T) {
	cfg := models.ModelConfig{
		ProviderConfig: &models.ProviderConfig{InternalCreditWeight: 5.0},
	}
	if ResolveICUWeight(cfg) != 5.0 {
		t.Fatal("expected explicit config weight 5.0")
	}
}

func TestResolveICUWeight_Local_4B(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "local",
		Metadata: &models.ModelMetadata{Parameters: 4_000_000_000},
	}
	if ResolveICUWeight(cfg) != 1.0 {
		t.Fatal("expected 1.0 for 4B local")
	}
}

func TestResolveICUWeight_Local_14B(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "local",
		Metadata: &models.ModelMetadata{Parameters: 14_000_000_000},
	}
	if ResolveICUWeight(cfg) != 1.5 {
		t.Fatalf("expected 1.5 for 14B, got %f", ResolveICUWeight(cfg))
	}
}

func TestResolveICUWeight_Local_20B(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "local",
		Metadata: &models.ModelMetadata{Parameters: 20_000_000_000},
	}
	if ResolveICUWeight(cfg) != 2.5 {
		t.Fatalf("expected 2.5 for 20B, got %f", ResolveICUWeight(cfg))
	}
}

func TestResolveICUWeight_Local_70B(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "local",
		Metadata: &models.ModelMetadata{Parameters: 70_000_000_000},
	}
	if ResolveICUWeight(cfg) != 4.0 {
		t.Fatalf("expected 4.0 for 70B, got %f", ResolveICUWeight(cfg))
	}
}

func TestResolveICUWeight_Cloud_Default(t *testing.T) {
	cfg := models.ModelConfig{Provider: "openai"}
	if ResolveICUWeight(cfg) != 1.0 {
		t.Fatal("expected default 1.0 for cloud without explicit weight")
	}
}

func TestResolveICUWeight_Local_NoMetadata(t *testing.T) {
	cfg := models.ModelConfig{Provider: "local"}
	if ResolveICUWeight(cfg) != 1.0 {
		t.Fatal("expected default 1.0 for local without metadata")
	}
}

func TestResolveContextLength_Metadata(t *testing.T) {
	cfg := &models.ModelConfig{
		Metadata: &models.ModelMetadata{ContextLength: 8192},
	}
	if resolveContextLength(cfg) != 8192 {
		t.Fatalf("expected 8192 from metadata, got %d", resolveContextLength(cfg))
	}
}

func TestResolveContextLength_MetadataWinsOverFragment(t *testing.T) {
	cfg := &models.ModelConfig{
		Name:     "deepseek-v3",
		Metadata: &models.ModelMetadata{ContextLength: 128000},
	}
	if resolveContextLength(cfg) != 128000 {
		t.Fatalf("expected metadata 128000 to win, got %d", resolveContextLength(cfg))
	}
}

func TestResolveContextLength_FragmentMatch(t *testing.T) {
	cfg := &models.ModelConfig{Name: "deepseek-v3"}
	if resolveContextLength(cfg) != 64_000 {
		t.Fatalf("expected 64K for deepseek-v3, got %d", resolveContextLength(cfg))
	}
}

func TestResolveContextLength_ProviderDefault(t *testing.T) {
	cfg := &models.ModelConfig{Name: "unknown-model", Provider: "nvidia"}
	if resolveContextLength(cfg) != 128_000 {
		t.Fatalf("expected 128K for nvidia, got %d", resolveContextLength(cfg))
	}
}

func TestResolveContextLength_NoMatch(t *testing.T) {
	cfg := &models.ModelConfig{Name: "unknown-model", Provider: "unknown"}
	if resolveContextLength(cfg) != 0 {
		t.Fatalf("expected 0 for unknown, got %d", resolveContextLength(cfg))
	}
}

func TestApplyMetadataDefaults_AllZero(t *testing.T) {
	cfg := &models.ModelConfig{
		Name:     "test-model",
		Provider: "nvidia",
	}
	ApplyMetadataDefaults(cfg)
	expectedTokens := 128_000 / 3
	expectedBudget := (128_000 - expectedTokens) * 2
	if cfg.MaxTokens != expectedTokens {
		t.Fatalf("expected max_tokens=%d, got %d", expectedTokens, cfg.MaxTokens)
	}
	if cfg.ContextBudget != expectedBudget {
		t.Fatalf("expected context_budget=%d (reserving %d for response), got %d",
			expectedBudget, expectedTokens, cfg.ContextBudget)
	}
}

func TestApplyMetadataDefaults_BudgetReservesResponseSpace(t *testing.T) {
	// A model with 8192 context and default max_tokens=2048 should get
	// budget = (8192-2048)*2 = 12288 chars, NOT 8192*2 = 16384.
	cfg := &models.ModelConfig{
		Name:     "qwen3.5-4b-instruct-q4_k_m.gguf",
		Provider: "local",
		Metadata: &models.ModelMetadata{ContextLength: 8192},
	}
	ApplyMetadataDefaults(cfg)
	expectedBudget := (8192 - cfg.MaxTokens) * 2
	if cfg.ContextBudget != expectedBudget {
		t.Fatalf("expected context_budget=%d (leaves %d tokens for response), got %d",
			expectedBudget, cfg.MaxTokens, cfg.ContextBudget)
	}
	// Verify the budget would keep prompt + response within 8192 tokens
	promptTokens := cfg.ContextBudget / 2 // ~2 chars/token
	if promptTokens+cfg.MaxTokens > 8192 {
		t.Fatalf("budget allows %d prompt tokens + %d response = %d, exceeds 8192 ctx",
			promptTokens, cfg.MaxTokens, promptTokens+cfg.MaxTokens)
	}
}

func TestApplyMetadataDefaults_ExplicitMaxTokensStillReserves(t *testing.T) {
	// User set max_tokens=512 explicitly, budget should compute from that.
	cfg := &models.ModelConfig{
		Name:       "small-context-model",
		Provider:   "local",
		MaxTokens:  512,
		Metadata:   &models.ModelMetadata{ContextLength: 4096},
	}
	ApplyMetadataDefaults(cfg)
	if cfg.MaxTokens != 512 {
		t.Fatalf("explicit max_tokens should not be overwritten, got %d", cfg.MaxTokens)
	}
	expectedBudget := (4096 - 512) * 2 // 7168
	if cfg.ContextBudget != expectedBudget {
		t.Fatalf("expected context_budget=%d (reserving %d response tokens), got %d",
			expectedBudget, cfg.MaxTokens, cfg.ContextBudget)
	}
}

func TestApplyMetadataDefaults_MaxTokensEqualsContext(t *testing.T) {
	// Edge case: max_tokens >= ctxLen. Should not produce negative budget.
	cfg := &models.ModelConfig{
		Name:       "test",
		Provider:   "local",
		MaxTokens:  8192,
		Metadata:   &models.ModelMetadata{ContextLength: 4096},
	}
	ApplyMetadataDefaults(cfg)
	if cfg.ContextBudget <= 0 {
		t.Fatalf("context_budget should not be negative or zero, got %d", cfg.ContextBudget)
	}
	// Fallback should kick in: ctxLen/2 * 2 = ctxLen = 4096
	if cfg.ContextBudget != 4096 {
		t.Fatalf("expected fallback budget 4096, got %d", cfg.ContextBudget)
	}
}

func TestApplyMetadataDefaults_NoOverwriteExisting(t *testing.T) {
	cfg := &models.ModelConfig{
		Name:           "test-model",
		Provider:       "nvidia",
		MaxTokens:      4096,
		ContextBudget:  8192,
	}
	ApplyMetadataDefaults(cfg)
	if cfg.MaxTokens != 4096 {
		t.Fatalf("existing max_tokens should not be overwritten, got %d", cfg.MaxTokens)
	}
	if cfg.ContextBudget != 8192 {
		t.Fatalf("existing context_budget should not be overwritten, got %d", cfg.ContextBudget)
	}
}
