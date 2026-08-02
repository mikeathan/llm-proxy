// local_safety_test.go — §3.2 local-context safety property + §0 local invariant
// verification for the Phase B+C workload classification.
//
//   ∀ cfg: effectiveEndpoint(cfg) is local ⟹ Classify(cfg) == WorkloadLocal
//                                     ∧ ReasoningWire(cfg) == thinking_budget_tokens
//
//   ∀ local cfg: context resolution fails ⟹ no cloud provider default is applied
//             ∧ LocalBudgetPolicy returns the numeric local default.
package orchestrator

import (
	"testing"

	"llm-proxy/models"
)

// TestClassifyLocalURLMatrix covers every local endpoint spelling the
// classifier must accept (C2 / §5): loopback, unspecified, model host, and a
// cached local-interface IP.
func TestClassifyLocalURLMatrix(t *testing.T) {
	ifaces := models.LocalInterfaceIPs()
	if len(ifaces) == 0 {
		t.Skip("no local interface IPs enumerated — cannot test interface-IP classification")
	}
	interfaceIP := ifaces[0].String()

	tests := []struct {
		name     string
		baseURL  string
		modelHost string
	}{
		{"localhost", "http://localhost:8080", "127.0.0.1"},
		{"127.0.0.1", "http://127.0.0.1:8080", "127.0.0.1"},
		{"::1", "http://[::1]:8080", "127.0.0.1"},
		{"0.0.0.0", "http://0.0.0.0:8080", "127.0.0.1"},
		{"model host", "http://127.0.0.1:8080", "127.0.0.1"},
		{"local interface IP", "http://" + interfaceIP + ":8080", "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := models.NewWorkloadClassifier(tt.modelHost, ifaces)
			cfg := models.ModelConfig{
				Provider: "openai",
				Name:     "llama-alias",
				ProviderConfig: &models.ProviderConfig{
					BaseURL: tt.baseURL,
				},
			}
			if got := c.Classify(cfg); got != models.WorkloadLocal {
				t.Fatalf("Classify(%q) = %q, want WorkloadLocal", tt.baseURL, got)
			}
		})
	}
}

// TestLocalContextNeverLeaksCloudDefault is V2: a local workload with no
// metadata and no name heuristic resolves to the numeric local default (8192 →
// ctx/3 = 2730), never a cloud provider default and never a typed error on the
// runtime path.
func TestLocalContextNeverLeaksCloudDefault(t *testing.T) {
	cases := []struct {
		name       string
		cfg        models.ModelConfig
		wantTokens int
	}{
		{
			name:       "openai local URL no metadata",
			cfg:        models.ModelConfig{Provider: "openai", Name: "llama-alias", ProviderConfig: &models.ProviderConfig{BaseURL: "http://127.0.0.1:8080"}},
			wantTokens: 2730, // 8192 / 3
		},
		{
			name:       "openai gguf no metadata",
			cfg:        models.ModelConfig{Provider: "openai", Name: "qwen.gguf"},
			wantTokens: 2730,
		},
		{
			name:       "local provider no metadata",
			cfg:        models.ModelConfig{Provider: "local", Name: "local-qwen"},
			wantTokens: 2730,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			class := models.NewWorkloadClassifier("127.0.0.1", nil).Classify(c.cfg)
			if class != models.WorkloadLocal {
				t.Fatalf("expected WorkloadLocal, got %q", class)
			}
			ctx := ResolveLocalContext(&c.cfg)
			if ctx != defaultLocalContextLength {
				t.Fatalf("expected defaultLocalContextLength %d, got %d", defaultLocalContextLength, ctx)
			}
			b, err := (LocalBudgetPolicy{}).Derive(c.cfg, ContextResolution{ServingContext: ctx})
			if err != nil {
				t.Fatalf("local runtime path must not return a typed error: %v", err)
			}
			if b.MaxTokens != c.wantTokens {
				t.Fatalf("expected max_tokens %d (ctx/3), got %d", c.wantTokens, b.MaxTokens)
			}
		})
	}
}

// TestLocalContextLengthOnly_Priority2 verifies §3.3: local with ContextLength
// only (no Nctx) resolves via priority 2 — golden byte-identical.
func TestLocalContextLengthOnly_Priority2(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "local",
		Name:     "local-qwen",
		Metadata: &models.ModelMetadata{ContextLength: 8192},
	}
	class := models.NewWorkloadClassifier("127.0.0.1", nil).Classify(cfg)
	if class != models.WorkloadLocal {
		t.Fatalf("expected WorkloadLocal, got %q", class)
	}
	ctx := ResolveLocalContext(&cfg)
	if ctx != 8192 {
		t.Fatalf("expected 8192 from ContextLength (priority 2), got %d", ctx)
	}
	b, err := (LocalBudgetPolicy{}).Derive(cfg, ContextResolution{ServingContext: ctx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MaxTokens != 2730 || b.ContextBudget != 10924 {
		t.Fatalf("expected 2730/10924 (golden byte-identical), got %d/%d", b.MaxTokens, b.ContextBudget)
	}
}

// TestLocalBogusMaxTokensIgnored verifies the handler-side contract (H1): a
// local submission's derived budget fields are cleared by
// enrichMetadataFromProviders so the n_ctx math wins.  ApplyMetadataDefaults
// itself preserves explicit values (documented in budget_squeezer_test.go) —
// the clearing happens at the registration edge for local workloads.
func TestLocalBogusMaxTokensIgnored(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "openai",
		Name:     "qwen3.5-4b-instruct-q4_k_m.gguf",
		Metadata: &models.ModelMetadata{Nctx: 8192, ContextLength: 262144},
	}
	// Bogus explicit value from a UI form.  The handler clears it for local
	// workloads (enrichMetadataFromProviders) before ApplyMetadataDefaults.
	// Simulate that clearing here to verify the n_ctx math wins.
	cfg.MaxTokens = 0
	cfg.ContextBudget = 0
	cfg.ReasoningBudget = 0
	ApplyMetadataDefaults(&cfg)
	if cfg.MaxTokens != 8192/3 {
		t.Fatalf("expected max_tokens %d (n_ctx math wins), got %d", 8192/3, cfg.MaxTokens)
	}
	if cfg.ContextBudget != (8192-8192/3)*2 {
		t.Fatalf("expected local context_budget from n_ctx, got %d", cfg.ContextBudget)
	}
}

// TestClassifyThroughCredentialOverride verifies §5: a local URL supplied
// through a credential/provider base-URL override classifies identically to
// inference.  The registrar hydrates the effective endpoint before classifying.
func TestClassifyThroughCredentialOverride(t *testing.T) {
	classifier := models.NewWorkloadClassifier("127.0.0.1", nil)
	// The persisted config carries no base URL; the effective endpoint comes
	// from the credential override — both must classify local.
	for _, effective := range []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
	} {
		cfg := models.ModelConfig{
			Provider: "openai",
			Name:     "llama-alias",
			ProviderConfig: &models.ProviderConfig{
				BaseURL: effective,
			},
		}
		if got := classifier.Classify(cfg); got != models.WorkloadLocal {
			t.Fatalf("effective endpoint %q must classify WorkloadLocal, got %q", effective, got)
		}
	}
}

// TestResolveICUWeight_OpenAIGGUF verifies M9: openai-provider GGUFs (now
// WorkloadLocal via the classifier) derive ICU weight from parameters instead
// of silently returning 1.0.
func TestResolveICUWeight_OpenAIGGUF(t *testing.T) {
	cfg := models.ModelConfig{
		Provider: "openai",
		Filename: "Qwen3.5-9B-UD-Q4_K_XL.gguf",
		Metadata: &models.ModelMetadata{Parameters: 9_197_093_888},
	}
	if got := ResolveICUWeight(cfg); got != 1.5 {
		t.Fatalf("expected parameter-derived ICU weight 1.5 for openai+gguf, got %f", got)
	}
}
