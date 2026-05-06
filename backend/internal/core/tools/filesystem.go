package tools

import (
	"context"
	"fmt"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"llm-proxy/internal/core/proxy"
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
	return ValidateFileSystemPath(path, isWrite, f.configProvider(ctx))
}

// ValidateFileSystemPath is a standalone validator for filesystem paths.
func ValidateFileSystemPath(path string, isWrite bool, cfg models.FileSystemGuardrailsConfig) (string, error) {
	if !cfg.Enabled {
		return "", fmt.Errorf("filesystem tools are disabled in configuration")
	}

	// 1. Check Read-Only Mode
	if isWrite && cfg.ReadOnly {
		return "", fmt.Errorf("filesystem is in read-only mode")
	}

	// 1.5 System Protection: Strictly block access to hidden files/folders and sensitive config files
	base := filepath.Base(path)
	isDot := path == "." || path == "./"
	
	// Check if any part of the path starts with "." (excluding "." and "..")
	parts := strings.Split(filepath.ToSlash(path), "/")
	hasHidden := false
	for _, p := range parts {
		if p != "." && p != ".." && strings.HasPrefix(p, ".") {
			hasHidden = true
			break
		}
	}

	if (!isDot && (hasHidden || strings.Contains(path, "..") || strings.Contains(path, "/."))) ||
		base == models.ConfigFilename || base == models.StateFilename ||
		base == models.LockFilename || base == models.SystemConfigFilename ||
		base == models.SecretsFilename || base == models.RegistryFilename {
		return "", fmt.Errorf("path access denied: restricted system file or directory (%s)", path)
	}

	logging.Debug("Validating filesystem path", "path", path, "isWrite", isWrite, "allowedRoots", cfg.AllowedPaths)

	// 1.6 Automatically resolve relative paths against the workspace root
	if !filepath.IsAbs(path) && len(cfg.AllowedPaths) > 0 {
		path = filepath.Join(cfg.AllowedPaths[0], path)
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

	// 4. Check Allowed Extensions (for both existing and new files)
	if len(cfg.AllowedExtensions) > 0 {
		// We check extensions for files only. 
		// If it's an existing directory, we skip extension check.
		fi, err := os.Stat(absPath)
		if err == nil && fi.IsDir() {
			return absPath, nil
		}

		ext := filepath.Ext(absPath)
		allowed := false
		for _, a := range cfg.AllowedExtensions {
			if strings.EqualFold(a, ext) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("file extension '%s' is not allowed", ext)
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
	names := make([]string, 0)
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
	return proxy.TruncateResult(string(content)), nil
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

