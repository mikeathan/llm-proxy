package tools

import (
	"encoding/json"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolManifestConsistency(t *testing.T) {
	// This test ensures that the Go constants in models/tools.go
	// stay in perfect sync with the tool_name property in our JSON manifests.
	
	manifestsDir := "manifests"

	tests := []struct {
		filename string
		expected string
	}{
		{"terminal.json", models.ToolTerminalExecute},
		{"communication.json", models.ToolNotifyUser},
		{"search.json", models.ToolInternetSearch},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			path := filepath.Join(manifestsDir, tt.filename)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read manifest %s: %v", tt.filename, err)
			}

			var manifest struct {
				ToolName string `json:"tool_name"`
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.filename, err)
			}

			if manifest.ToolName != tt.expected {
				t.Errorf("Manifest %s mismatch: JSON has %q, Go constant has %q. Please synchronize models/tools.go",
					tt.filename, manifest.ToolName, tt.expected)
			}
		})
	}
}

// TestTerminalManifest_NoCdPersistencePromise guards the terminal tool contract:
// the description must NOT claim that 'cd' persists across calls (the persistent
// shell runs each command from the workspace root). It must instead direct callers
// to chain 'cd subdir && ...' in one command, so plan-and-execute and other callers
// never produce cd-dependent multi-step plans.
func TestTerminalManifest_NoCdPersistencePromise(t *testing.T) {
	_, description, err := LoadManifestAsTool("terminal", models.ToolTerminalExecute)
	if err != nil {
		t.Fatalf("LoadManifestAsTool failed: %v", err)
	}
	if strings.Contains(description, "will persist") {
		t.Errorf("terminal description must not promise cd persistence, got: %q", description)
	}
	if !strings.Contains(description, "workspace root") {
		t.Errorf("terminal description must state commands run in the workspace root, got: %q", description)
	}
	if !strings.Contains(description, "chain `cd subdir") {
		t.Errorf("terminal description must direct chaining `cd subdir && ...`, got: %q", description)
	}
}
