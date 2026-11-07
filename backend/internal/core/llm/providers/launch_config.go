package providers

import (
	"fmt"
	"io/fs"
	"os"
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

// localProvider is the provider ID of models run by the local llama server.
const localProvider = "local"

// LocateModelFile finds the one file called name anywhere under dir, for a model kept in a subfolder of the model folder. It returns ""
// when name is not a plain file name, when dir cannot be read, when no file has that name, and when two do: guessing between two files
// would start the wrong model.
func LocateModelFile(dir, name string) string {
	if dir == "" || name == "" || name != filepath.Base(name) {
		return ""
	}
	found := ""
	matches := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			matches++
		}
		return nil
	})
	if matches != 1 {
		return ""
	}
	return found
}

// FindInModelDir returns where the model file at path is when it is not where the model folder says: path itself when the file is there,
// else the one file of that name in a subfolder of dir, else path unchanged, so a failure names the place that was looked in first.
func FindInModelDir(dir, path string) string {
	if dir == "" || fileExists(path) {
		return path
	}
	if located := LocateModelFile(dir, filepath.Base(path)); located != "" {
		return located
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// NormalizeModelConfig fills in what a registry entry leaves out. A local model's path is the model folder joined with its file name; when
// no file is there, the file is looked for in the subfolders of the model folder, so a model kept in one is found after a restart as well
// as when it was added. A path the entry already holds is kept as it is.
func NormalizeModelConfig(baseDir string, cfg models.ModelConfig) models.ModelConfig {
	out := cfg
	local := out.Provider == localProvider
	if local && out.Path != "" && !fileExists(out.Path) {
		out.Path = "" // the file moved since the path was saved: look for it again
	}
	if out.Path == "" {
		out.Path = ResolveModelFile(baseDir, out)
		if local {
			out.Path = FindInModelDir(baseDir, out.Path)
		}
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

	sanitized := SanitizeArgs(cfg.Args)

	args := []string{"-m", cfg.Path, "--host", host, "--port", fmt.Sprint(cfg.Port)}
	return append(args, sanitized...)
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
