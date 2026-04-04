package llm

import (
	"llm-proxy/models"
	"path/filepath"
)

// Common helper functions for configuration and resolution.

func resolveModelFile(baseDir string, m models.ModelConfig) string {
	if m.Path != "" && filepath.IsAbs(m.Path) {
		return m.Path
	}
	fname := m.Filename
	if fname == "" && m.Path != "" {
		fname = filepath.Base(m.Path)
	}
	if fname == "" {
		return ""
	}
	if filepath.IsAbs(fname) {
		return fname
	}
	if baseDir != "" {
		return filepath.Join(baseDir, fname)
	}
	return fname
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
	out.Path = resolveModelFile(modelDir, model)
	return out
}

func normalizeModelConfig(baseDir string, cfg models.ModelConfig) models.ModelConfig {
	out := cfg
	if out.Path == "" {
		out.Path = resolveModelFile(baseDir, out)
	}
	if out.Filename == "" && out.Path != "" {
		out.Filename = filepath.Base(out.Path)
	}
	return out
}
