package llm

import (
	"llm-proxy/models"
	"testing"
)

func TestConfigModelFromConfig_EnvironmentMerging(t *testing.T) {
	cfg := &models.Config{
		Server: models.ServerConfig{
			Environment: map[string]string{
				"GLOBAL_VAR": "global",
				"OVERRIDE":   "global",
			},
		},
		Providers: map[string]models.ProviderItem{
			"local": {
				Environment: map[string]string{
					"PROVIDER_VAR": "provider",
					"OVERRIDE":     "provider",
				},
			},
			"custom": {
				Environment: map[string]string{
					"CUSTOM_VAR": "custom",
				},
			},
		},
	}

	tests := []struct {
		name     string
		model    models.ModelConfig
		expected map[string]string
	}{
		{
			name: "Model with explicit provider 'local'",
			model: models.ModelConfig{
				Name:     "test-1",
				Provider: "local",
				Environment: map[string]string{
					"MODEL_VAR": "model",
					"OVERRIDE":  "model",
				},
			},
			expected: map[string]string{
				"GLOBAL_VAR":   "global",
				"PROVIDER_VAR": "provider",
				"MODEL_VAR":    "model",
				"OVERRIDE":     "model", // Model should win
			},
		},
		{
			name: "Model with empty provider (should fallback to 'local')",
			model: models.ModelConfig{
				Name:     "test-2",
				Provider: "",
				Environment: map[string]string{
					"MODEL_VAR": "model",
				},
			},
			expected: map[string]string{
				"GLOBAL_VAR":   "global",
				"PROVIDER_VAR": "provider", // Should pick up local vars
				"MODEL_VAR":    "model",
				"OVERRIDE":     "provider", // Provider wins over Global
			},
		},
		{
			name: "Model with custom provider",
			model: models.ModelConfig{
				Name:     "test-3",
				Provider: "custom",
			},
			expected: map[string]string{
				"GLOBAL_VAR": "global",
				"CUSTOM_VAR": "custom",
				"OVERRIDE":   "global", // Custom didn't override
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := configModelFromConfig(cfg, tt.model)

			if len(result.Environment) != len(tt.expected) {
				t.Errorf("expected %d env vars, got %d", len(tt.expected), len(result.Environment))
			}

			for k, v := range tt.expected {
				if result.Environment[k] != v {
					t.Errorf("for key %s: expected %s, got %s", k, v, result.Environment[k])
				}
			}
		})
	}
}
