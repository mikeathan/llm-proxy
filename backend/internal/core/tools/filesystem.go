package tools

import (
	"fmt"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
)

// FileSystemTools provides secure file operations.
type FileSystemTools struct {
	configProvider func() models.FileSystemGuardrailsConfig
}

func NewFileSystemTools(provider func() models.FileSystemGuardrailsConfig) *FileSystemTools {
	return &FileSystemTools{configProvider: provider}
}

func (f *FileSystemTools) ValidatePath(path string, isWrite bool) (string, error) {
	cfg := f.configProvider()
	if !cfg.Enabled {
		return "", fmt.Errorf("filesystem tools are disabled in configuration")
	}

	// 1. Check Read-Only Mode
	if isWrite && cfg.ReadOnly {
		return "", fmt.Errorf("filesystem is in read-only mode")
	}

	// 2. Resolve and Validate Path (Jailing)
	absPath, err := IsSecurePath(path, cfg.AllowedPaths)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(absPath)

	// 3. Check Blocked Filenames
	for _, blocked := range cfg.BlockedFilenames {
		if filename == blocked || strings.HasPrefix(filename, blocked) {
			return "", fmt.Errorf("access to sensitive file '%s' is blocked by guardrails", filename)
		}
	}

	// 4. Check Allowed Extensions (only for files)
	if len(cfg.AllowedExtensions) > 0 {
		fi, err := os.Stat(absPath)
		if err == nil && !fi.IsDir() {
			ext := filepath.Ext(absPath)
			allowed := false
			for _, a := range cfg.AllowedExtensions {
				if a == ext {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("file extension '%s' is not allowed", ext)
			}
		}
	}

	return absPath, nil
}

func (f *FileSystemTools) ListDirectory(path string) ([]string, error) {
	absPath, err := f.ValidatePath(path, false)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		// Filter out blocked files from listing too
		isBlocked := false
		for _, b := range f.configProvider().BlockedFilenames {
			if name == b {
				isBlocked = true
				break
			}
		}
		if isBlocked {
			continue
		}

		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	return names, nil
}

func (f *FileSystemTools) ReadFile(path string) (string, error) {
	absPath, err := f.ValidatePath(path, false)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (f *FileSystemTools) WriteFile(path string, content string) error {
	absPath, err := f.ValidatePath(path, true)
	if err != nil {
		return err
	}

	cfg := f.configProvider()

	// Check File Size Quota
	if cfg.MaxFileSizeKB > 0 && (len(content)/1024) > cfg.MaxFileSizeKB {
		return fmt.Errorf("file size exceeds quota (max %d KB)", cfg.MaxFileSizeKB)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0644)
}

// IsSecurePath checks if a path is within the allowed roots.
func IsSecurePath(path string, allowedRoots []string) (string, error) {
	absLocal, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	for _, root := range allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		// Robust prefix check: handle same path or subpath correctly
		if absLocal == absRoot || strings.HasPrefix(absLocal, absRoot+string(filepath.Separator)) {
			return absLocal, nil
		}
	}

	return "", fmt.Errorf("path access denied: outside of authorized workspaces")
}
