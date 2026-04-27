package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"llm-proxy/models"
	"llm-proxy/internal/platform/network"
)

// ResolveHost is a bridge to the centralized network utility.
func ResolveHost(host string) string {
	return network.ResolveHost(host)
}

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

func BuildLaunchArgs(cfg models.ModelConfig, host string) []string {
	if host == "" {
		host = "127.0.0.1"
	}
	args := []string{"-m", cfg.Path, "--host", host, "--port", fmt.Sprint(cfg.Port)}
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

func ValidateModelPath(path string) error {
	if path == "" {
		return fmt.Errorf("model path is empty; cannot start local model")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("model file not found at %q: %w", path, err)
	}
	return nil
}
