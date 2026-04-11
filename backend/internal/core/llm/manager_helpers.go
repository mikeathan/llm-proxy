package llm

import (
	"fmt"
	"path/filepath"

	"llm-proxy/models"
)

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

func buildLaunchArgs(cfg models.ModelConfig) []string {
	args := []string{"-m", cfg.Path, "--port", fmt.Sprint(cfg.Port)}
	return append(args, sanitizeArgs(cfg.Args)...)
}

func sanitizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skipNext := false
	for i := 0; i < len(args); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		arg := args[i]
		if arg == "--n-batch" {
			skipNext = true
			continue
		}
		out = append(out, arg)
	}
	return out
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

	if p, ok := m.providers[pName]; ok {
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
