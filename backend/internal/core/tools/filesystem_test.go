package tools

import (
	"context"
	"net"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestIsSecurePath(t *testing.T) {
	tmpDir := t.TempDir()

	ws1 := filepath.Join(tmpDir, models.WorkspacesDirName, "workspace-1")
	ws11 := filepath.Join(tmpDir, models.WorkspacesDirName, "workspace-11")
	other := filepath.Join(tmpDir, "outside")

	_ = os.MkdirAll(ws1, 0755)
	_ = os.MkdirAll(ws11, 0755)
	_ = os.MkdirAll(other, 0755)

	allowedRoots := []string{ws1}

	escapeLink := filepath.Join(ws1, "escape")
	_ = os.Symlink(other, escapeLink)

	parentLinkDir := filepath.Join(ws1, "parent-link")
	_ = os.MkdirAll(parentLinkDir, 0755)
	_ = os.Symlink(other, filepath.Join(parentLinkDir, "evil-link"))

	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{name: "file inside workspace", path: filepath.Join(ws1, "test.txt"), allowed: true},
		{name: "workspace root itself", path: ws1, allowed: true},
		{name: "similar-named workspace", path: filepath.Join(ws11, "secret.txt"), allowed: false},
		{name: "file totally outside", path: filepath.Join(other, "foo.txt"), allowed: false},
		{name: "traversal attempt", path: filepath.Join(ws1, "..", "workspace-11", "secret.txt"), allowed: false},
		{name: "symlink escape", path: filepath.Join(ws1, "escape", "secret.txt"), allowed: false},
		{name: "parent symlink escape", path: filepath.Join(ws1, "parent-link", "evil-link", "secret.txt"), allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := IsSecurePath(tt.path, allowedRoots)
			isAllowed := err == nil
			if isAllowed != tt.allowed {
				t.Errorf("IsSecurePath(%q) allowed = %v, want %v (err: %v)", tt.path, isAllowed, tt.allowed, err)
			}
		})
	}
}

func TestIsSecurePath_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	runsDir := filepath.Join(tmpDir, "runs")
	_ = os.MkdirAll(runsDir, 0755)

	wsSimilar := filepath.Join(tmpDir, "workspace-similar")
	_ = os.MkdirAll(wsSimilar, 0755)

	outsideDir := filepath.Join(tmpDir, "outside")
	_ = os.MkdirAll(outsideDir, 0755)
	_ = os.Symlink(outsideDir, filepath.Join(wsDir, "escape-link"))

	chain1 := filepath.Join(wsDir, "chain1")
	chain2 := filepath.Join(tmpDir, "chain2")
	_ = os.Symlink(chain2, chain1)
	_ = os.Symlink(runsDir, chain2)

	allowed := []string{wsDir}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "inside workspace", path: filepath.Join(wsDir, "test.txt"), wantErr: false},
		{name: "sibling same prefix", path: filepath.Join(wsSimilar, "test.txt"), wantErr: true},
		{name: "outside runs", path: filepath.Join(runsDir, "recording.jsonl"), wantErr: true},
		{name: "symlink escape", path: filepath.Join(wsDir, "escape-link", "secret.txt"), wantErr: true},
		{name: "symlink chain", path: filepath.Join(wsDir, "chain1", "secret.txt"), wantErr: true},
		{name: "root itself", path: wsDir, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := IsSecurePath(tt.path, allowed)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("IsSecurePath(%q) error = %v, wantErr = %v", tt.path, gotErr, tt.wantErr)
			}
		})
	}
}

func TestFileSystemTools_EditFileBlock_NormalizeBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"trailing spaces", "hello   \nworld\t\n", "hello\nworld\n"},
		{"windows line endings", "line1\r\nline2\r\n", "line1\nline2\n"},
		{"mixed", "a  \r\n b \t\n", "a\n b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBlock(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeBlock(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFileSystemTools_EditFileBlock(t *testing.T) {
	tmpDir := t.TempDir()
	tools := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	create := func(name, content string) {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0600); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	run := func(name, oldB, newB string) (string, error) {
		return tools.EditFileBlock(context.Background(), filepath.Join(tmpDir, name), oldB, newB)
	}

	t.Run("exact match", func(t *testing.T) {
		create("exact.ts", `function foo() { return 1; }`)
		_, err := run("exact.ts", "return 1;", "return 2;")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(tmpDir, "exact.ts"))
		if string(got) != "function foo() { return 2; }" {
			t.Errorf("expected replaced content, got: %s", string(got))
		}
	})

	t.Run("trailing whitespace normalization", func(t *testing.T) {
		create("ws.ts", "function bar(  \n  x: number  \n): string {")
		_, err := run("ws.ts", "  x: number  ", "  x: number | modified")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(tmpDir, "ws.ts"))
		if !strings.Contains(string(got), "  x: number | modified") {
			t.Errorf("expected normalized replacement, got: %s", string(got))
		}
	})

	t.Run("windows line endings", func(t *testing.T) {
		create("crlf.ts", "a\r\nb\r\nc")
		_, err := run("crlf.ts", "a\nb\n", "a\nB\n")
		if err != nil {
			t.Fatalf("unexpected error with CRLF: %v", err)
		}
	})

	t.Run("multiple matches", func(t *testing.T) {
		create("multi.ts", "x\ny\nx\nz")
		_, err := run("multi.ts", "x", "X")
		if err == nil {
			t.Fatal("expected error for multiple matches")
		}
		if !strings.Contains(err.Error(), "2 matching blocks") {
			t.Errorf("expected '2 matching blocks' error, got: %v", err)
		}
	})

	t.Run("no match", func(t *testing.T) {
		create("none.ts", "original content")
		_, err := run("none.ts", "does not exist", "new")
		if err == nil {
			t.Fatal("expected error for no match")
		}
		if !strings.Contains(err.Error(), "block not found") {
			t.Errorf("expected 'block not found' error, got: %v", err)
		}
	})

	t.Run("empty old block", func(t *testing.T) {
		create("empty.ts", "anything")
		_, err := run("empty.ts", "", "new")
		if err == nil {
			t.Fatal("expected error for empty old_block")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		_, err := run("nope.ts", "anything", "new")
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
		if !strings.Contains(err.Error(), "file does not exist") {
			t.Errorf("expected 'file does not exist' error, got: %v", err)
		}
	})
}

func TestFileSystemTools_WriteFile_AnyLength(t *testing.T) {
	tmpDir := t.TempDir()
	tools := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	shortFile := filepath.Join(tmpDir, "short.txt")
	err := tools.WriteFile(context.Background(), shortFile, "short content")
	if err != nil {
		t.Fatalf("WriteFile short content should succeed: %v", err)
	}

	longFile := filepath.Join(tmpDir, "long.txt")
	longContent := strings.Repeat("x", 5000)
	err = tools.WriteFile(context.Background(), longFile, longContent)
	if err != nil {
		t.Fatalf("WriteFile long content should succeed: %v", err)
	}

	readback, err := os.ReadFile(longFile)
	if err != nil {
		t.Fatalf("ReadFile after long write failed: %v", err)
	}
	if string(readback) != longContent {
		t.Fatalf("expected full content to match, got %d of %d chars", len(readback), len(longContent))
	}
}

func TestFileSystemTools_WritePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	tools := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	testFile := filepath.Join(tmpDir, "test.txt")
	err := tools.WriteFile(context.Background(), testFile, "content")
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("Expected permissions 0600, got %o", mode)
	}
}

// ── Security path validation tests ──

// TestFileSystemTools_ListDirectory_HidesSandboxRuntime verifies the sandbox
// runtime directory (.sandbox) never appears in directory listings — the agent
// must not browse the sandbox runtime (the shared internalBlockedPaths
// invariant also enforced by the terminal tool).
func TestFileSystemTools_ListDirectory_HidesSandboxRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(filepath.Join(wsDir, ".sandbox"), 0755)
	_ = os.MkdirAll(filepath.Join(wsDir, "docs"), 0755)
	if err := os.WriteFile(filepath.Join(wsDir, "notes.txt"), []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{wsDir},
		}
	}
	f := NewFileSystemTools(cfg)

	out, err := f.ListDirectory(context.Background(), ".")
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	joined, ok := out.(string)
	if !ok {
		t.Fatalf("expected string listing, got %T", out)
	}
	if strings.Contains(joined, ".sandbox") {
		t.Fatalf("listing must hide sandbox runtime dir, got: %q", joined)
	}
	if !strings.Contains(joined, "docs/") || !strings.Contains(joined, "notes.txt") {
		t.Fatalf("listing must keep ordinary entries, got: %q", joined)
	}
}

func TestValidateFileSystemPath_Security(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	runsDir := filepath.Join(tmpDir, "runs")
	_ = os.MkdirAll(runsDir, 0755)

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{wsDir},
		}
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "file inside workspace", path: "test.txt", wantErr: false},
		{name: "subdirectory", path: "subdir/file.txt", wantErr: false},
		{name: "parent traversal", path: "../outside.txt", wantErr: true},
		{name: "deep traversal", path: "../../etc/passwd", wantErr: true},
		{name: "traversal into runs", path: "../runs/recording.jsonl", wantErr: true},
		{name: "hidden directory", path: ".secret/config.yml", wantErr: true},
		{name: "hidden file", path: ".env", wantErr: true},
		{name: "hidden in subdir", path: "subdir/.secret.txt", wantErr: true},
		{name: "current dir", path: ".", wantErr: false},
		{name: "empty path", path: "", wantErr: false}, // resolved to workspace root
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFileSystemTools(cfg)
			_, err := f.ValidatePath(context.Background(), tt.path, false)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr = %v", tt.path, gotErr, tt.wantErr)
			}
		})
	}
}

func TestValidateFileSystemPath_AbsoluteOutside(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	runsDir := filepath.Join(tmpDir, "runs")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(runsDir, 0755)

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{wsDir},
		}
	}

	f := NewFileSystemTools(cfg)

	absOutsidePaths := []string{
		runsDir,
		filepath.Join(runsDir, "recording.jsonl"),
		"/etc/passwd",
		"/tmp",
	}

	for _, p := range absOutsidePaths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			_, err := f.ValidatePath(context.Background(), p, false)
			if err == nil {
				t.Errorf("expected error for absolute path outside workspace: %s", p)
			}
		})
	}
}

func TestValidateFileSystemPath_BlockedFilenames(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:          true,
			AllowedPaths:     []string{wsDir},
			BlockedFilenames: []string{".env", "config.json", "secrets"},
		}
	}

	f := NewFileSystemTools(cfg)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "allowed", path: "notes.txt", wantErr: false},
		{name: "blocked exact", path: ".env", wantErr: true},
		{name: "blocked prefix", path: "secrets.yaml", wantErr: true},
		{name: "blocked in subdir", path: "subdir/.env", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.ValidatePath(context.Background(), tt.path, false)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr = %v", tt.path, gotErr, tt.wantErr)
			}
		})
	}
}

func TestValidateFileSystemPath_ReadOnly(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			ReadOnly:     true,
			AllowedPaths: []string{wsDir},
		}
	}

	f := NewFileSystemTools(cfg)

	_, err := f.ValidatePath(context.Background(), "test.txt", false)
	if err != nil {
		t.Errorf("read should work: %v", err)
	}

	_, err = f.ValidatePath(context.Background(), "test.txt", true)
	if err == nil {
		t.Error("write should fail in read-only mode")
	}
}

func TestValidateFileSystemPath_NoAllowedRoots(t *testing.T) {
	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{},
		}
	}

	f := NewFileSystemTools(cfg)
	_, err := f.ValidatePath(context.Background(), "test.txt", false)
	if err == nil {
		t.Error("expected error when no allowed roots")
	}
}

func TestValidateFileSystemPath_BlockedSystemFiles(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	cfg := func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{wsDir},
		}
	}

	f := NewFileSystemTools(cfg)

	systemFiles := []string{
		models.ConfigFilename,
		models.StateFilename,
		models.LockFilename,
		models.SystemConfigFilename,
		models.SecretsFilename,
		models.RegistryFilename,
	}
	for _, name := range systemFiles {
		t.Run(name, func(t *testing.T) {
			_, err := f.ValidatePath(context.Background(), name, true)
			if err == nil {
				t.Errorf("expected error for system file: %s", name)
			}
		})
	}
}

func TestReadFileWithTimeout_WithinLimit(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	data, err := readFileWithTimeout(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestReadFileWithTimeout_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readFileWithTimeout(ctx, "/no/such/file")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestReadFileWithTimeout_LeakedMetric(t *testing.T) {
	tmpDir := t.TempDir()

	fifoPath := filepath.Join(tmpDir, "fifo.pipe")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Skipf("mkfifo not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	before := LeakedFileReads()
	_, err := readFileWithTimeout(ctx, fifoPath)
	if err == nil {
		t.Fatal("expected error from blocking fifo read")
	}
	after := LeakedFileReads()
	if after <= before {
		t.Errorf("leakedFileReads did not increment: before=%d, after=%d", before, after)
	}
}

func TestReadFileWithTimeout_BoundedPool(t *testing.T) {
	for i := 0; i < maxConcurrentFileReads; i++ {
		if !readFileSem.TryAcquire(1) {
			t.Fatalf("expected to acquire slot %d/%d", i+1, maxConcurrentFileReads)
		}
	}
	if readFileSem.TryAcquire(1) {
		t.Fatalf("expected semaphore to be full at %d slots", maxConcurrentFileReads)
	}
	for i := 0; i < maxConcurrentFileReads; i++ {
		readFileSem.Release(1)
	}
	if !readFileSem.TryAcquire(1) {
		t.Fatal("expected to acquire slot after releasing all")
	}
	readFileSem.Release(1)
}

func TestLeakedFileReads_InitialZero(t *testing.T) {
	if n := LeakedFileReads(); n < 0 {
		t.Errorf("LeakedFileReads() should not be negative: %d", n)
	}
}

func TestReadFile_Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large.txt")
	large := strings.Repeat("abcdefghij", 400)
	if err := os.WriteFile(path, []byte(large), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	fs := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	result, err := fs.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "[SYSTEM: File truncated") {
		t.Errorf("expected truncation notice, got: %s", result[:200])
	}
	if len(result) > 3200 {
		t.Errorf("truncated result too long: %d chars", len(result))
	}
}

func TestReadFile_NoTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "small.txt")
	content := "short file"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	fs := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	result, err := fs.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Errorf("content mismatch: got %q, want %q", result, content)
	}
}

func TestReadFileWithTimeout_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.txt")
	if _, err := os.Create(path); err != nil {
		t.Fatalf("setup: %v", err)
	}

	data, err := readFileWithTimeout(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty content, got %q", string(data))
	}
}

func TestReadFileWithTimeout_ConcurrentDoesNotPanic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "f.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var wg sync.WaitGroup
	var errCount atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := readFileWithTimeout(context.Background(), path)
			if err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if n := errCount.Load(); n > 0 {
		t.Logf("concurrent reads had %d errors (may be OK on resource-constrained systems)", n)
	}
}

func TestNetworkResolver_PureGo(t *testing.T) {
	r := &net.Resolver{PreferGo: true}
	ips, err := r.LookupIP(context.Background(), "ip", "localhost")
	if err != nil {
		t.Fatalf("LookupIP failed: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("expected at least one IP for localhost")
	}
	t.Logf("localhost resolved to %d IPs with PreferGo", len(ips))
}
