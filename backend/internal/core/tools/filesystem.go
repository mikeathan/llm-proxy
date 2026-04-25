package tools

import (
	"context"
	"fmt"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
)

const secureFileMode = 0600

// FileSystemTools provides secure file operations.
type FileSystemTools struct {
	configProvider func(ctx context.Context) models.FileSystemGuardrailsConfig
}

func NewFileSystemTools(provider func(ctx context.Context) models.FileSystemGuardrailsConfig) *FileSystemTools {
	return &FileSystemTools{configProvider: provider}
}

func (f *FileSystemTools) Config(ctx context.Context) models.FileSystemGuardrailsConfig {
	return f.configProvider(ctx)
}

func (f *FileSystemTools) ValidatePath(ctx context.Context, path string, isWrite bool) (string, error) {
	cfg := f.configProvider(ctx)
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

func (f *FileSystemTools) ListDirectory(ctx context.Context, path string) ([]string, error) {
	absPath, err := f.ValidatePath(ctx, path, false)
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
		for _, b := range f.configProvider(ctx).BlockedFilenames {
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

func (f *FileSystemTools) ReadFile(ctx context.Context, path string) (string, error) {
	absPath, err := f.ValidatePath(ctx, path, false)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (f *FileSystemTools) WriteFile(ctx context.Context, path string, content string) error {
	absPath, err := f.ValidatePath(ctx, path, true)
	if err != nil {
		return err
	}

	cfg := f.configProvider(ctx)

	// Check File Size Quota
	if cfg.MaxFileSizeKB > 0 && (len(content)/1024) > cfg.MaxFileSizeKB {
		return fmt.Errorf("file size exceeds quota (max %d KB)", cfg.MaxFileSizeKB)
	}


	// Create directory if it doesn't exist
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), secureFileMode)
}

// IsSecurePath checks if a path is within the allowed roots.
func IsSecurePath(path string, allowedRoots []string) (string, error) {
	absLocal, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Resolve symlinks to prevent jail escape.
	// If the file exists, resolve it fully.
	if resolved, err := filepath.EvalSymlinks(absLocal); err == nil {
		absLocal = resolved
	} else if os.IsNotExist(err) {
		// If the file doesn't exist, we must still resolve its parent to prevent
		// creating a file inside a symlinked directory that points outside the jail.
		parent := filepath.Dir(absLocal)
		if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
			absLocal = filepath.Join(resolvedParent, filepath.Base(absLocal))
		}
	}

	for _, root := range allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		// Resolve symlinks in roots too to ensure canonical comparison
		if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
			absRoot = resolvedRoot
		}

		// Robust prefix check: handle same path or subpath correctly
		if absLocal == absRoot || strings.HasPrefix(absLocal, absRoot+string(filepath.Separator)) {
			return absLocal, nil
		}
	}

	return "", fmt.Errorf("path access denied: outside of authorized workspaces")
}
