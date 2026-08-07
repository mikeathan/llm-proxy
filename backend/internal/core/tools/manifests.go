package tools

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"llm-proxy/models"
)

//go:embed manifests/*.json
var manifestFS embed.FS

// ToolManifest is the generic structure for any tool definition.
type ToolManifest struct {
	ToolName    string          `json:"tool_name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Runtime     json.RawMessage `json:"runtime,omitempty"`
	Guardrails  json.RawMessage `json:"guardrails"`
}

// LoadManifest generic helper to load a manifest for a specific tool.
// It tries to load from the disk first (to support dev-syncing), and falls back
// to the embedded standard manifests in production.
func LoadManifest(toolKey string, target any) error {
	var data []byte
	var err error

	// 1. Try Disk first (for live dev updates)
	// We anchor this to the source file's location to be robust across different execution roots.
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		sourceDir := filepath.Dir(filename)
		diskPath := filepath.Join(sourceDir, "manifests", toolKey+".json")
		data, err = os.ReadFile(diskPath)
	}

	// 2. Fallback to Embedded FS (Production)
	if err != nil || len(data) == 0 {
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

// LoadManifestAsTool loads a JSON manifest and converts it to an LLM-ready tool definition.
func LoadManifestAsTool(toolKey string, toolName string) (map[string]any, string, error) {
	var data []byte
	var err error

	// Try Disk first (for live dev updates)
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		sourceDir := filepath.Dir(filename)
		diskPath := filepath.Join(sourceDir, "manifests", toolKey+".json")
		data, err = os.ReadFile(diskPath)
	}

	// Fallback to Embedded FS (Production)
	if err != nil || len(data) == 0 {
		embedPath := fmt.Sprintf("manifests/%s.json", toolKey)
		data, err = manifestFS.ReadFile(embedPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read %s manifest: %w", toolKey, err)
		}
	}

	var m ToolManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, "", err
	}

	// 2. Parse parameters into map[string]any
	params := make(map[string]any)
	description := m.Description

	// 3. Look for per-tool overrides in the 'runtime' section
	if m.Runtime != nil {
		var runtime map[string]struct {
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		if err := json.Unmarshal(m.Runtime, &runtime); err == nil {
			if toolCfg, ok := runtime[toolName]; ok {
				if toolCfg.Description != "" {
					description = toolCfg.Description
				}
				if len(toolCfg.Parameters) > 0 {
					if err := json.Unmarshal(toolCfg.Parameters, &params); err != nil {
						return nil, "", fmt.Errorf("failed to parse %s runtime parameters: %w", toolName, err)
					}
					// If we found specific runtime parameters, return early
					return params, description, nil
				}
			}
		}
	}

	// 4. Fallback to global manifest parameters
	if len(m.Parameters) > 0 {
		if err := json.Unmarshal(m.Parameters, &params); err != nil {
			return nil, "", fmt.Errorf("failed to parse %s parameters: %w", toolKey, err)
		}
	}

	if params == nil {
		params = make(map[string]any)
	}

	return params, description, nil
}

var (
	defaultGuardrailsOnce sync.Once
	defaultGuardrails     models.AgentGuardrailsConfig
)

// GetDefaultGuardrails returns the merged guardrail defaults from the static
// manifests. The result is computed once and cached (P3) — manifests do not
// change at runtime — and deep-copied per call so no caller can mutate the
// shared cache.
func GetDefaultGuardrails() models.AgentGuardrailsConfig {
	defaultGuardrailsOnce.Do(func() {
		cfg := models.AgentGuardrailsConfig{}
		cfg.Global.BlockSecrets = true

		_ = LoadManifest("terminal", &cfg.Terminal)
		_ = LoadManifest("search", &cfg.Search)
		_ = LoadManifest("communication", &cfg.Communication)
		_ = LoadManifest("filesystem", &cfg.FileSystem)
		_ = LoadManifest("network", &cfg.Network)
		_ = LoadManifest("security", &cfg.Global)

		defaultGuardrails = cfg
	})

	// Deep copy so callers cannot mutate the shared cache through the returned
	// value's slices/maps.
	data, err := json.Marshal(defaultGuardrails)
	if err != nil {
		return defaultGuardrails
	}
	var copy models.AgentGuardrailsConfig
	if err := json.Unmarshal(data, &copy); err != nil {
		return defaultGuardrails
	}
	return copy
}
