package llm

import (
	"context"
	"testing"
	"time"

	"llm-proxy/models"
)

func fingerprintClassifier(cfg models.ModelConfig) models.WorkloadClass {
	return models.NewWorkloadClassifier("", nil).Classify(cfg)
}

func qwenBehindURL() models.ModelConfig {
	return models.ModelConfig{
		Name:           "Qwen3.6 35B A3B",
		Provider:       models.ProviderOpenAI,
		Filename:       "Qwen3.6 35B A3B",
		ProviderConfig: &models.ProviderConfig{BaseURL: "https://models.example.net/v1"},
		Metadata:       &models.ModelMetadata{ContextLength: 16384, Nctx: 16384},
		MaxTokens:      8192,
		ContextBudget:  8192,
		WorkloadClass:  models.WorkloadCloud,
	}
}

func TestWithServingFingerprint(t *testing.T) {
	entry := models.ProviderModelInfo{ID: "Qwen3.6 35B A3B", Meta: &models.ModelMeta{Serving: models.ServingLlamaCpp, Nctx: 16384}}

	t.Run("a llama.cpp entry makes the model local and re-derives its budgets", func(t *testing.T) {
		got, changed := withServingFingerprint(qwenBehindURL(), entry, fingerprintClassifier)
		if !changed {
			t.Fatal("expected a change")
		}
		if got.WorkloadClass != models.WorkloadLocal || got.Metadata.Serving != models.ServingLlamaCpp {
			t.Errorf("class %q serving %q, want local / llamacpp", got.WorkloadClass, got.Metadata.Serving)
		}
		if want := 16384 / 3; got.MaxTokens != want {
			t.Errorf("max_tokens = %d, want %d (ctx/3)", got.MaxTokens, want)
		}
		if got.ContextBudget <= 8192 {
			t.Errorf("context_budget = %d, want the local policy's larger window", got.ContextBudget)
		}
	})

	t.Run("an explicit reasoning budget survives and the cloud tool-call default is cleared", func(t *testing.T) {
		cfg := qwenBehindURL()
		cfg.ReasoningBudget = 1234
		cfg.ToolCallFormat = "native"
		got, _ := withServingFingerprint(cfg, entry, fingerprintClassifier)
		if got.ReasoningBudget != 1234 {
			t.Errorf("reasoning budget = %d, want the explicit 1234", got.ReasoningBudget)
		}
		if got.ToolCallFormat != "" {
			t.Errorf("tool call format = %q, want it re-derived for a local workload", got.ToolCallFormat)
		}
	})

	t.Run("a model that already carries the fingerprint is left alone", func(t *testing.T) {
		cfg := qwenBehindURL()
		cfg.Metadata.Serving = models.ServingLlamaCpp
		if _, changed := withServingFingerprint(cfg, entry, fingerprintClassifier); changed {
			t.Error("unexpected change")
		}
	})

	t.Run("an entry without the fingerprint changes nothing", func(t *testing.T) {
		cloud := models.ProviderModelInfo{ID: "Qwen3.6 35B A3B", ContextLength: 1048576}
		got, changed := withServingFingerprint(qwenBehindURL(), cloud, fingerprintClassifier)
		if changed || got.WorkloadClass != models.WorkloadCloud {
			t.Errorf("changed=%v class=%q, want unchanged cloud", changed, got.WorkloadClass)
		}
	})
}

// A server that is not up at startup is read again; one that answered is not.
func TestRetryUntilAnswered(t *testing.T) {
	a, b := models.ModelConfig{Name: "a"}, models.ModelConfig{Name: "b"}
	calls := map[string]int{}
	try := func(cfg models.ModelConfig) bool {
		calls[cfg.Name]++
		return cfg.Name == "a" || calls[cfg.Name] >= 3 // b answers on its third try
	}
	retryUntilAnswered(context.Background(), []time.Duration{0, time.Millisecond, time.Millisecond, time.Millisecond}, []models.ModelConfig{a, b}, try)
	if calls["a"] != 1 || calls["b"] != 3 {
		t.Errorf("tries a=%d b=%d, want a once and b until it answered (3)", calls["a"], calls["b"])
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := 0
	retryUntilAnswered(ctx, []time.Duration{0, time.Hour}, []models.ModelConfig{b}, func(models.ModelConfig) bool { n++; return false })
	if n > 1 {
		t.Errorf("a cancelled context kept retrying (%d tries)", n)
	}
}
