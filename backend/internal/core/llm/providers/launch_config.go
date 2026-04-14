package providers

import (
	"fmt"
	"path/filepath"
	"llm-proxy/models"
)

func ResolveModelFile(baseDir string, m models.ModelConfig) string {
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

func NormalizeModelConfig(baseDir string, cfg models.ModelConfig) models.ModelConfig {
	out := cfg
	if out.Path == "" {
		out.Path = ResolveModelFile(baseDir, out)
	}
	if out.Filename == "" && out.Path != "" {
		out.Filename = filepath.Base(out.Path)
	}
	return out
}

func BuildLaunchArgs(cfg models.ModelConfig) []string {
	args := []string{"-m", cfg.Path, "--port", fmt.Sprint(cfg.Port)}
	return append(args, SanitizeArgs(cfg.Args)...)
}

func SanitizeArgs(args []string) []string {
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
