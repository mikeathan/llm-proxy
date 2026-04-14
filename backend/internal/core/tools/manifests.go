package tools

import (
	"embed"
	"encoding/json"
	"fmt"
	"llm-proxy/models"
	"os"
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
// It tries to load from the disk first (if root is provided) to support dev-syncing,
// and falls back to the embedded standard manifests.
func LoadManifest(root string, toolKey string, target any) error {
	var data []byte
	var err error

	// 1. Try Disk first (for live dev updates)
	if root != "" {
		diskPath := fmt.Sprintf("%s/backend/internal/core/tools/manifests/%s.json", root, toolKey)
		data, err = os.ReadFile(diskPath)
	}

	// 2. Fallback to Embedded FS
	if err != nil || root == "" {
		embedPath := fmt.Sprintf("manifests/%s.json", toolKey)
		data, err = manifestFS.ReadFile(embedPath)
		if err != nil {
			return fmt.Errorf("failed to read %s manifest (embed): %w", toolKey, err)
		}
	}

	var m ToolManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse %s manifest: %w", toolKey, err)
	}

	if err := json.Unmarshal(m.Guardrails, target); err != nil {
		return fmt.Errorf("failed to parse %s guardrails: %w", toolKey, err)
	}

	return nil
}

// GetDefaultGuardrails loads all guardrail configurations from their respective manifests.
func GetDefaultGuardrails(root string) models.AgentGuardrailsConfig {
	var cfg models.AgentGuardrailsConfig
	cfg.Global.BlockSecrets = true

	_ = LoadManifest(root, "terminal", &cfg.Terminal)
	_ = LoadManifest(root, "search", &cfg.Search)
	_ = LoadManifest(root, "communication", &cfg.Communication)
	_ = LoadManifest(root, "filesystem", &cfg.FileSystem)
	_ = LoadManifest(root, "security", &cfg.Global)

	return cfg
}

// SaveManifest updates the on-disk manifest file with new guardrails.
// This is used for dev-syncing UI changes back to the source repository.
func SaveManifest(root string, toolKey string, guardrails any) error {
	path := fmt.Sprintf("%s/backend/internal/core/tools/manifests/%s.json", root, toolKey)
	
	// 1. Read existing manifest to preserve other fields (name, version, etc)
	var manifest struct {
		ToolName    string          `json:"tool_name"`
		Version     string          `json:"version"`
		Description string          `json:"description"`
		Guardrails  json.RawMessage `json:"guardrails"`
		Runtime     json.RawMessage `json:"runtime,omitempty"`
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read manifest for writing: %w", err)
	}

	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}

	// 2. Replace Guardrails
	newData, err := json.MarshalIndent(guardrails, "", "  ")
	if err != nil {
		return err
	}
	manifest.Guardrails = newData

	// 3. Write back
	finalData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, finalData, 0644)
}
