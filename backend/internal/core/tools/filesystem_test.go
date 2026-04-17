package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSecurePath(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a simulated structure
	ws1 := filepath.Join(tmpDir, "workspaces", "workspace-1")
	ws11 := filepath.Join(tmpDir, "workspaces", "workspace-11")
	other := filepath.Join(tmpDir, "outside")
	
	_ = os.MkdirAll(ws1, 0755)
	_ = os.MkdirAll(ws11, 0755)
	_ = os.MkdirAll(other, 0755)

	allowedRoots := []string{ws1}

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
			name:    "relative path in workspace",
			path:    "./test.txt", // This depends on CWD, but we should test absolute resolution
			allowed: false, // Usually rejected if not absolute OR not matching root
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
