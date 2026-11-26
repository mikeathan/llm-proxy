package orchestrator

import (
	"testing"

	"llm-proxy/models"
)

func TestServingContextFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"ctx-size flag", []string{"-m", "x.gguf", "--ctx-size", "8192"}, 8192},
		{"short -c flag", []string{"-c", "4096"}, 4096},
		{"context-size flag", []string{"--context-size", "2048"}, 2048},
		{"0 means the file's own context", []string{"--ctx-size", "0"}, 0},
		{"no ctx flag", []string{"-m", "x.gguf", "--threads", "8"}, 0},
		{"missing value", []string{"--ctx-size"}, 0},
		{"nil args", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ServingContextFromArgs(c.args); got != c.want {
				t.Fatalf("expected %d, got %d", c.want, got)
			}
		})
	}
}

// The window a local model is launched with (--ctx-size) is its serving window until the running server reports its
// own. Without it a model whose server had not answered yet was sized by the file's training context: on 2026-11-25 an
// Inkling launched with --ctx-size 8192 was given the budget of its 1,048,576-token training window.
func TestResolveLocalContext_LaunchArgs(t *testing.T) {
	train := &models.ModelMetadata{ContextLength: 1_048_576}
	cases := []struct {
		name string
		cfg  models.ModelConfig
		want int
	}{
		{"--ctx-size beats the training context", models.ModelConfig{Args: []string{"--ctx-size", "8192"}, Metadata: train}, 8192},
		{"the server's own report beats --ctx-size", models.ModelConfig{Args: []string{"--ctx-size", "8192"}, Metadata: &models.ModelMetadata{ContextLength: 1_048_576, Nctx: 16384}}, 16384},
		{"--ctx-size with no metadata at all", models.ModelConfig{Args: []string{"-c", "4096"}}, 4096},
		{"--ctx-size 0 falls back to the training context", models.ModelConfig{Args: []string{"--ctx-size", "0"}, Metadata: &models.ModelMetadata{ContextLength: 32768}}, 32768},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveLocalContext(&c.cfg); got != c.want {
				t.Fatalf("expected %d, got %d", c.want, got)
			}
		})
	}
}

// A model this app launches gets its window from its launch args until its server answers: a stored n_ctx is either a
// training-context guess the add form recorded (vertex 2026-11-26: Ornith at --ctx-size 8192 stored 262144) or a report
// from args since changed, and it outranked --ctx-size. A model whose args set no window, a cloud model and a remote
// llama.cpp host keep what they have.
func TestDropStoredServingContext(t *testing.T) {
	stored := func() *models.ModelMetadata { return &models.ModelMetadata{ContextLength: 262144, Nctx: 262144} }
	cases := []struct {
		name     string
		cfg      models.ModelConfig
		wantNctx int
	}{
		{"a launched model with --ctx-size drops it", models.ModelConfig{Provider: models.ProviderLocal, Args: []string{"--ctx-size", "8192"}, Metadata: stored()}, 0},
		{"a launched model without --ctx-size keeps it", models.ModelConfig{Provider: models.ProviderLocal, Args: []string{"--threads", "8"}, Metadata: stored()}, 262144},
		{"--ctx-size 0 sets no window, so it is kept", models.ModelConfig{Provider: models.ProviderLocal, Args: []string{"--ctx-size", "0"}, Metadata: stored()}, 262144},
		{"a remote llama.cpp host keeps its probed window", models.ModelConfig{Provider: "openai", Args: []string{"--ctx-size", "8192"}, Metadata: stored()}, 262144},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := c.cfg.Metadata
			DropStoredServingContext(&c.cfg)
			if got := c.cfg.Metadata.Nctx; got != c.wantNctx {
				t.Fatalf("Nctx = %d, want %d", got, c.wantNctx)
			}
			if before.Nctx != 262144 {
				t.Error("the metadata shared with other copies of the config must not be changed")
			}
			if c.cfg.Metadata.ContextLength != 262144 {
				t.Error("the training context is kept")
			}
		})
	}
	t.Run("no metadata is left alone", func(t *testing.T) {
		cfg := models.ModelConfig{Provider: models.ProviderLocal, Args: []string{"--ctx-size", "8192"}}
		DropStoredServingContext(&cfg)
		if cfg.Metadata != nil {
			t.Errorf("metadata = %+v, want nil", cfg.Metadata)
		}
	})
	t.Run("the budget then follows --ctx-size", func(t *testing.T) {
		cfg := models.ModelConfig{Provider: models.ProviderLocal, WorkloadClass: models.WorkloadLocal, Args: []string{"--ctx-size", "8192"}, Metadata: stored()}
		DropStoredServingContext(&cfg)
		ApplyMetadataDefaults(&cfg)
		if cfg.ContextBudget != 21848 || cfg.MaxTokens != 2730 {
			t.Errorf("budget %d, max tokens %d; want 21848 and 2730 (an 8192-token window)", cfg.ContextBudget, cfg.MaxTokens)
		}
	})
}
