package tools

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed manifests/*.json
var manifestFS embed.FS

// ToolManifest is the generic structure for any tool definition.
type ToolManifest struct {
	ToolName    string          `json:"tool_name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Guardrails  json.RawMessage `json:"guardrails"`
}

// LoadManifest generic helper to load a manifest for a specific tool.
func LoadManifest(toolKey string, target any) error {
	path := fmt.Sprintf("manifests/%s.json", toolKey)
	data, err := manifestFS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s manifest: %w", toolKey, err)
	}

	var m ToolManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse config manifest: %w", err)
	}

	if err := json.Unmarshal(m.Guardrails, target); err != nil {
		return fmt.Errorf("failed to parse %s guardrails: %w", toolKey, err)
	}

	return nil
}
