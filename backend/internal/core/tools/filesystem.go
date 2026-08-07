package tools

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/semaphore"
)

const secureFileMode = 0600

const (
	maxConcurrentFileReads = 10
	fileReadTimeout        = 30 * time.Second
)

var (
	readFileSem     = semaphore.NewWeighted(maxConcurrentFileReads)
	leakedFileReads atomic.Int64
)

// LeakedFileReads returns the count of file reads that were abandoned
// (goroutine leaked) due to context cancellation or timeout on stuck mounts.
func LeakedFileReads() int64 {
	return leakedFileReads.Load()
}

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
	return validateFilePath(path, isWrite, f.configProvider(ctx))
}

// ValidateFileSystemPath is a standalone validator for filesystem paths.
// Used by the guardrail engine for policy enforcement including extension
// checks.  The tool engine uses validateFilePath internally so that
// guardrail- approved extension blocks are respected (not re-rejected).
func ValidateFileSystemPath(path string, isWrite bool, cfg models.FileSystemGuardrailsConfig) (string, error) {
	absPath, err := validateFilePath(path, isWrite, cfg)
	if err != nil {
		return "", err
	}

	// Extension check — policy layer only.  Not duplicated in validateFilePath
	// so the tool engine does not re-reject what the guardrail already approved.
	if len(cfg.AllowedExtensions) > 0 {
		fi, err := os.Stat(absPath)
		if err == nil && fi.IsDir() {
			return absPath, nil
		}

		ext := filepath.Ext(absPath)
		if !isWrite && ext == "" {
			return absPath, nil
		}

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

// validateFilePath resolves and validates a path without checking file
// extensions (enforced at guardrail level).  Used by the tool engine so
// guardrail approvals for extension blocks are respected.
func validateFilePath(path string, isWrite bool, cfg models.FileSystemGuardrailsConfig) (string, error) {
	if !cfg.Enabled {
		return "", fmt.Errorf("filesystem tools are disabled in configuration")
	}

	if isWrite && cfg.ReadOnly {
		return "", fmt.Errorf("filesystem is in read-only mode")
	}

	base := filepath.Base(path)
	isDot := path == "." || path == "./"

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

	if !filepath.IsAbs(path) && len(cfg.AllowedPaths) > 0 {
		path = filepath.Join(cfg.AllowedPaths[0], path)
	}

	absPath, err := IsSecurePath(path, cfg.AllowedPaths)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(absPath)

	for _, blocked := range cfg.BlockedFilenames {
		if filename == blocked || strings.HasPrefix(filename, blocked) {
			return "", fmt.Errorf("access to sensitive file '%s' is blocked by guardrails", filename)
		}
	}

	return absPath, nil
}

func (f *FileSystemTools) ListDirectory(ctx context.Context, path string) (any, error) {
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
	//  Phase 3: Structural Truncation for Directory Listing
	return proxy.TruncateLines(names, 30), nil
}

// readFileWithTimeout reads a file using os.ReadFile in a goroutine, racing
// against ctx cancellation. On stuck NFS mounts the underlying syscall ignores
// context — the goroutine leaks but we return early. Tracked via leakedFileReads.
// Concurrent reads are bounded by maxConcurrentFileReads semaphore.
func readFileWithTimeout(ctx context.Context, path string) ([]byte, error) {
	if err := readFileSem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer readFileSem.Release(1)

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := os.ReadFile(path)
		ch <- result{data, err}
	}()

	select {
	case <-ctx.Done():
		leakedFileReads.Add(1)
		return nil, ctx.Err()
	case r := <-ch:
		return r.data, r.err
	}
}

func (f *FileSystemTools) ReadFile(ctx context.Context, path string) (string, error) {
	absPath, err := f.ValidatePath(ctx, path, false)
	if err != nil {
		return "", err
	}

	content, err := readFileWithTimeout(ctx, absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}

	//  Phase 3: Structural Truncation for ReadFile (3000 chars)
	raw := string(content)
	if len(raw) <= 3000 {
		return raw, nil
	}

	head := raw[:1500]
	tail := raw[len(raw)-1500:]
	return fmt.Sprintf("%s\n\n... [SYSTEM: File truncated (%d bytes total). Use 'read_range' or 'grep' to access the middle] ...\n\n%s",
		head, len(raw), tail), nil
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

func (f *FileSystemTools) AppendFile(ctx context.Context, path string, content string) error {
	absPath, err := f.ValidatePath(ctx, path, true)
	if err != nil {
		return err
	}

	cfg := f.configProvider(ctx)

	fi, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf(models.ToolMissingForAppendMsg, models.ToolFileWrite, models.ToolFileAppend)
	}

	totalSize := fi.Size() + int64(len(content))
	if cfg.MaxFileSizeKB > 0 && (totalSize/1024) > int64(cfg.MaxFileSizeKB) {
		return fmt.Errorf("file size would exceed quota (max %d KB)", cfg.MaxFileSizeKB)
	}

	fh, err := os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY, secureFileMode)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteString(content)
	return err
}

func (f *FileSystemTools) EditFileBlock(ctx context.Context, path string, oldBlock string, newBlock string) (string, error) {
	absPath, err := f.ValidatePath(ctx, path, false)
	if err != nil {
		return "", err
	}

	if oldBlock == "" {
		return "", fmt.Errorf("old_block cannot be empty")
	}

	raw, err := readFileWithTimeout(ctx, absPath)
	if err != nil {
		return "", fmt.Errorf(models.ToolMissingForEditMsg, models.ToolFileWrite)
	}

	fileContent := string(raw)
	normFile := normalizeBlock(fileContent)
	normOld := normalizeBlock(oldBlock)

	idx := strings.Index(normFile, normOld)
	if idx < 0 {
		return "", fmt.Errorf("block not found in file. Read the file to verify the exact content, then try again with the correct old_block")
	}

	nextIdx := strings.Index(normFile[idx+1:], normOld)
	if nextIdx >= 0 {
		return "", fmt.Errorf("found 2 matching blocks. Include more surrounding context (3-5 lines) in old_block to make the match unique")
	}

	// Build the replacement by preserving the original content's whitespace.
	newContent := buildReplacement(fileContent, oldBlock, newBlock, idx, normFile, normOld)

	cfg := f.configProvider(ctx)
	totalSize := len(newContent)
	if cfg.MaxFileSizeKB > 0 && (totalSize/1024) > cfg.MaxFileSizeKB {
		return "", fmt.Errorf("resulting file would exceed quota (max %d KB)", cfg.MaxFileSizeKB)
	}

	if err := storage.WriteAtomic(absPath, filepath.Base(absPath)+"-*.tmp", []byte(newContent), storage.ClassUserContent); err != nil {
		return "", fmt.Errorf("atomic write: %w", err)
	}
	return fmt.Sprintf("Replaced 1 block (%d bytes).", len(oldBlock)), nil
}

// normalizeBlock strips trailing whitespace per line and normalizes line endings
// so the model doesn't need to match whitespace precisely.
func normalizeBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Strip \r from \r\n, then trim trailing spaces/tabs.
		line = strings.TrimRight(strings.TrimRight(line, "\r"), " \t")
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// buildReplacement replaces the normalized-match region in the original content
// with newBlock, preserving original whitespace outside the replaced block.
func buildReplacement(content, oldBlock, newBlock string, normIdx int, normContent, normOld string) string {
	origPos := 0
	normPos := 0
	origLen := len(content)
	normOldLen := len(normOld)
	matchEnd := normIdx + normOldLen

	start := -1

	for origPos < origLen && normPos < matchEnd {
		oc := content[origPos]
		nc := normContent[normPos]

		if oc == nc {
			if normPos == normIdx {
				start = origPos
			}
			normPos++
			origPos++
		} else if oc == ' ' || oc == '\t' || oc == '\r' {
			origPos++
		} else {
			origPos++
		}
	}

	if start < 0 {
		return content
	}

	end := origPos

	return content[:start] + newBlock + content[end:]
}
