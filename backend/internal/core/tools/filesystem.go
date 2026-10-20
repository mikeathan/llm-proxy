package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sync/semaphore"
)

const secureFileMode = 0600

// File reads are bounded by the caller's context (FilesystemToolTimeout, applied
// in tool_exec.go) and by this semaphore; there is no second timer.
const maxConcurrentFileReads = 10

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

// ErrInstructionFileReadOnly is the refusal to write the workspace's AGENTS.md. That file holds the operator's
// instructions for the workspace and is read into every run, so an agent able to rewrite it could plant instructions
// (say, from a web page it read) that outlive the conversation. Reading stays allowed. It is a security boundary: the
// agent loop never offers it for approval, and the text tells the model what to do instead.
var ErrInstructionFileReadOnly = errors.New("path access denied: AGENTS.md holds the operator's instructions for this workspace and is read-only to the assistant; tell the user what to change and let them edit it in the Files view")

// isInstructionFile reports whether path names the instruction file, in any case and at any depth.
func isInstructionFile(path string) bool {
	return strings.EqualFold(filepath.Base(path), models.RulesFilename)
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
	if isWrite && isInstructionFile(path) {
		return "", ErrInstructionFileReadOnly
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
	// The resolved path too: a link with another name that points at AGENTS.md is still AGENTS.md.
	if isWrite && isInstructionFile(absPath) {
		return "", ErrInstructionFileReadOnly
	}

	if m := blockedFilename(absPath, effectiveBlockedFilenames(cfg.BlockedFilenames)); m != nil {
		return "", fmt.Errorf("path access denied: access to sensitive file '%s' is blocked", *m)
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
		// Filter out blocked files from listing too.  The effective list merges
		// user-configured BlockedFilenames with the internal invariant paths
		// (.sandbox etc.), so a sandbox runtime directory never appears in a
		// listing and no per-invariant code is needed.
		if blockedFilename(name, effectiveBlockedFilenames(f.configProvider(ctx).BlockedFilenames)) != nil {
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

	return truncatedFileRead(string(content)), nil
}

// fileNoteReserve is the room, in the encoded result, kept for the note that opens a truncated read and the line that
// marks the omitted middle.
const fileNoteReserve = 260

// truncatedFileRead is what a read with no range returns: the whole file when it fits the result cap once encoded,
// else a note, the start and the end of the file. The note comes first and the whole result is sized to the cap, so
// the second cut at appendToolResult (which drops the middle of an over-long result) cannot remove it.
func truncatedFileRead(raw string) string {
	if encodedLen(raw) <= proxy.MaxReturnChars {
		return raw
	}
	half := (proxy.MaxReturnChars - fileNoteReserve) / 2
	head := raw[:fittingPrefix(raw, half)]
	tail := raw[len(raw)-fittingSuffix(raw[len(head):], half):]
	return fmt.Sprintf("[SYSTEM: File truncated (%d bytes total). This is its start and its end; read the rest with read_file offset=%d limit=<bytes> (bytes).]\n%s\n... [middle omitted] ...\n%s",
		len(raw), len(head), head, tail)
}

// defaultRangeBytes is how much a ranged read returns when it names no limit; rangeNoteReserve leaves room, in the
// encoded result, for the header and the trailing note; rangeReadCap bounds what one piece reads from disk, however
// large the file or the limit asked for (a piece never shows more than the result cap anyway).
const (
	defaultRangeBytes = proxy.MaxReturnChars
	rangeNoteReserve  = 120
	rangeReadCap      = 4 * proxy.MaxReturnChars
)

// ReadFileRange reads a file in pieces: up to limit bytes from byte offset (offset 0 and the default limit when
// either is omitted). With neither it is ReadFile. The piece is cut on character boundaries and sized so the result,
// once JSON-encoded, fits the result cap, so it is not cut again; the note at its end says where to continue. Only the
// piece is read from disk, not the whole file.
func (f *FileSystemTools) ReadFileRange(ctx context.Context, path string, offset, limit *int) (string, error) {
	if offset == nil && limit == nil {
		return f.ReadFile(ctx, path)
	}
	start, want := 0, defaultRangeBytes
	if offset != nil {
		start = *offset
	}
	if limit != nil {
		want = *limit
	}
	if start < 0 {
		return "", fmt.Errorf("offset must not be negative")
	}
	if want <= 0 {
		return "", fmt.Errorf("limit must be greater than zero")
	}
	absPath, err := f.ValidatePath(ctx, path, false)
	if err != nil {
		return "", err
	}
	w, err := readWindow(ctx, absPath, start, min(want, rangeReadCap))
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}
	return w.render(want), nil
}

// fileWindow is a piece of a file: data holds the bytes that start at byte start of a file total bytes long.
type fileWindow struct {
	total, start int
	data         string
}

// readWindow reads up to want bytes at start, plus the few bytes that finish a character cut by want. It is
// readFileWithTimeout for a piece: same semaphore, same early return when ctx ends.
func readWindow(ctx context.Context, path string, start, want int) (fileWindow, error) {
	if err := readFileSem.Acquire(ctx, 1); err != nil {
		return fileWindow{}, err
	}
	defer readFileSem.Release(1)

	type result struct {
		w   fileWindow
		err error
	}
	ch := make(chan result, 1)
	go func() {
		w, err := readWindowAt(path, start, want)
		ch <- result{w, err}
	}()
	select {
	case <-ctx.Done():
		leakedFileReads.Add(1)
		return fileWindow{}, ctx.Err()
	case r := <-ch:
		return r.w, r.err
	}
}

func readWindowAt(path string, start, want int) (fileWindow, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileWindow{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fileWindow{}, err
	}
	w := fileWindow{total: int(info.Size()), start: start}
	if start >= w.total {
		return w, nil
	}
	buf := make([]byte, min(want+utf8.UTFMax, w.total-start))
	n, err := file.ReadAt(buf, int64(start))
	if err != nil && !errors.Is(err, io.EOF) {
		return fileWindow{}, err
	}
	w.data = string(buf[:n])
	return w, nil
}

// render is the piece as the model sees it: a header with the real byte range, the text (start moved forward to a
// character start, end cut back to one, and shortened to fit the result cap once encoded), and a note saying where
// to continue or that the file ended. want is the limit asked for.
func (w fileWindow) render(want int) string {
	if w.total > 0 && w.start >= w.total {
		return fmt.Sprintf("[offset %d is past the end: the file has %d bytes]", w.start, w.total)
	}
	skip := 0
	for skip < len(w.data) && !utf8.RuneStart(w.data[skip]) {
		skip++
	}
	start, data := w.start+skip, w.data[skip:]
	text := data[:charStart(data, min(len(data), min(want, rangeReadCap)))]
	if text == "" && data != "" { // the limit is smaller than one character: give that character
		_, size := utf8.DecodeRuneInString(data)
		text = data[:size]
	}
	text = text[:fittingPrefix(text, proxy.MaxReturnChars-rangeNoteReserve)]
	end := start + len(text)
	note := "\n[end of file]"
	if end < w.total {
		note = fmt.Sprintf("\n[more: read_file offset=%d]", end)
	}
	return fmt.Sprintf("[bytes %d-%d of %d]\n%s%s", start, end, w.total, text, note)
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
	absPath, err := f.ValidatePath(ctx, path, true) // an edit rewrites the file: it is a write
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
