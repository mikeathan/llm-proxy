package llm

import (
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
)

func resolveModelFile(baseDir string, m models.ModelConfig) string {
	return providers.ResolveModelFile(baseDir, m)
}

func configModelFromConfig(cfg *models.Config, model models.ModelConfig) models.ModelConfig {
	var args []string
	if len(model.Args) == 0 {
		args = append([]string(nil), cfg.Server.DefaultArgs...)
	} else {
		args = append([]string(nil), model.Args...)
	}

	env := make(map[string]string)
	for k, v := range cfg.Server.Environment {
		env[k] = v
	}

	pName := model.Provider
	if pName == "" {
		pName = "local"
	}

	if p, ok := cfg.Providers[pName]; ok {
		for k, v := range p.Environment {
			env[k] = v
		}
	}

	for k, v := range model.Environment {
		env[k] = v
	}

	modelDir := ""
	if local, ok := cfg.Providers["local"]; ok {
		modelDir = local.ModelDir
	}

	out := model
	out.Args = args
	out.Environment = env
	out.Path = providers.ResolveModelFile(modelDir, model)
	return out
}

func normalizeModelConfig(baseDir string, cfg models.ModelConfig) models.ModelConfig {
	return providers.NormalizeModelConfig(baseDir, cfg)
}

func buildLaunchArgs(cfg models.ModelConfig) []string {
	return providers.BuildLaunchArgs(cfg)
}

func sanitizeArgs(args []string) []string {
	return providers.SanitizeArgs(args)
}

func (m *LLMRuntimeManager) enrichModelLocked(cfg models.ModelConfig) models.ModelConfig {
	env := make(map[string]string)
	for k, v := range m.serverEnv {
		env[k] = v
	}

	pName := cfg.Provider
	if pName == "" {
		pName = "local"
	}

	if p, ok := m.registrar.ListConfigs()[pName]; ok {
		for k, v := range p.Environment {
			env[k] = v
		}
	}

	for k, v := range cfg.Environment {
		env[k] = v
	}

	out := cfg
	out.Environment = env
	return out
}
