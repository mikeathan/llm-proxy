package tools

import (
	"context"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSecurePath(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a simulated structure
	ws1 := filepath.Join(tmpDir, models.WorkspacesDirName, "workspace-1")
	ws11 := filepath.Join(tmpDir, models.WorkspacesDirName, "workspace-11")
	other := filepath.Join(tmpDir, "outside")
	
	_ = os.MkdirAll(ws1, 0755)
	_ = os.MkdirAll(ws11, 0755)
	_ = os.MkdirAll(other, 0755)

	allowedRoots := []string{ws1}

	// Create symlink escape
	escapeLink := filepath.Join(ws1, "escape")
	_ = os.Symlink(other, escapeLink)

	// Create symlink in parent directory
	parentLinkDir := filepath.Join(ws1, "parent-link")
	_ = os.MkdirAll(parentLinkDir, 0755)
	_ = os.Symlink(other, filepath.Join(parentLinkDir, "evil-link"))

	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{
			name:    "file inside workspace",
			path:    filepath.Join(ws1, "test.txt"),
			allowed: true,
		},
		{
			name:    "workspace root itself",
			path:    ws1,
			allowed: true,
		},
		{
			name:    "file in similar-named workspace (JAILBREAK ATTEMPT)",
			path:    filepath.Join(ws11, "secret.txt"),
			allowed: false, // Should be false because of ws1 prefix vs ws11
		},
		{
			name:    "file totally outside",
			path:    filepath.Join(other, "foo.txt"),
			allowed: false,
		},
		{
			name:    "traversal attempt",
			path:    filepath.Join(ws1, "..", "workspace-11", "secret.txt"),
			allowed: false,
		},
		{
			name:    "symlink escape attempt (SEC-H1)",
			path:    filepath.Join(ws1, "escape", "secret.txt"),
			allowed: false,
		},
		{
			name:    "parent symlink escape attempt (SEC-H1)",
			path:    filepath.Join(ws1, "parent-link", "evil-link", "secret.txt"),
			allowed: false,
		},
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

	// Helper to create a file.
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

	// Short content should succeed.
	shortFile := filepath.Join(tmpDir, "short.txt")
	err := tools.WriteFile(context.Background(), shortFile, "short content")
	if err != nil {
		t.Fatalf("WriteFile short content should succeed: %v", err)
	}

	// Long content (over 800 chars) should also succeed — no artificial limit.
	longFile := filepath.Join(tmpDir, "long.txt")
	longContent := strings.Repeat("x", 5000)
	err = tools.WriteFile(context.Background(), longFile, longContent)
	if err != nil {
		t.Fatalf("WriteFile long content should succeed: %v", err)
	}

	// Verify the full content was written.
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

	// Mode() returns FileMode. On Unix, the bottom 9 bits are permissions.
	// We expect 0600 (-rw-------)
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("Expected permissions 0600, got %o", mode)
	}
}
