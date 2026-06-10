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

func TestFileSystemTools_WriteFile_MaxLength(t *testing.T) {
	tmpDir := t.TempDir()
	tools := NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{
			Enabled:      true,
			AllowedPaths: []string{tmpDir},
		}
	})

	testFile := filepath.Join(tmpDir, "long.txt")

	// Content under 800 chars should succeed.
	err := tools.WriteFile(context.Background(), testFile, "short content")
	if err != nil {
		t.Fatalf("WriteFile under limit should succeed: %v", err)
	}

	// Content over 800 chars should fail with the expected error.
	longContent := strings.Repeat("x", 801)
	err = tools.WriteFile(context.Background(), testFile, longContent)
	if err == nil {
		t.Fatal("WriteFile over limit should return an error")
	}
	if !strings.Contains(err.Error(), "content too long") {
		t.Errorf("expected 'content too long' error, got: %v", err)
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
