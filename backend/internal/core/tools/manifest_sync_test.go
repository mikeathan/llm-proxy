package tools

import (
	"encoding/json"
	"llm-proxy/models"
	"os"
	"path/filepath"
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
