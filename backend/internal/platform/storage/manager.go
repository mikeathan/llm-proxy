package storage

import (
	"fmt"
	"path/filepath"
)

// DataManager orchestrates the 3-Tier storage model.
type DataManager struct {
	rootDir string

	// Config (System/Infrastructure)
	systemStore *Store[SystemConfig]

	// Secrets (Credentials)
	secretStore *Store[SecretData]

	// Registry (Models/Providers State)
	registryStore *Store[RegistryData]
}

// SystemConfig represents the infrastructure-level settings (config.json)
type SystemConfig struct {
	Server struct {
		Bind            string `json:"bind"`
		ModelHost       string `json:"model_host"`
		IdleTimeoutSecs int    `json:"idle_timeout_seconds"`
		PrimaryModel    string `json:"primary_model,omitempty"`
		FallbackModel   string `json:"fallback_model,omitempty"`
	} `json:"server"`

	// Local Infrastructure settings
	Local struct {
		LlamaServerBinary string   `json:"llama_server_binary"`
		ModelDir          string   `json:"model_dir"`
		DefaultArgs       []string `json:"default_args"`
	} `json:"local"`

	WorkspacesDir string `json:"workspaces_dir"`
}

// RegistryData represents the dynamic application state (registry.json)
type RegistryData struct {
	Providers map[string]ProviderRegistryEntry `json:"providers"`
	Catalogue []ModelRegistryEntry             `json:"catalogue"`
	MCPServers []MCPServerRegistryEntry        `json:"mcp_servers"`
}

type ProviderRegistryEntry struct {
	Type                string `json:"type"`
	DefaultCredentialID string `json:"default_credential_id,omitempty"`
	BaseURL             string `json:"base_url,omitempty"`
}

type ModelRegistryEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderID   string `json:"provider_id"`
	ModelID      string `json:"model_id"` // Provider specific ID/Filename
	CredentialID string `json:"credential_id,omitempty"`
}

type MCPServerRegistryEntry struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// SecretData represents the encrypted/isolated vault (secrets.json)
type SecretData struct {
	Version      int                     `json:"version"`
	ProviderKeys map[string][]SecretEntry `json:"provider_keys"`
}

type SecretEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// NewDataManager initializes the storage root and creates the 3-tier stores.
func NewDataManager(rootDir string) (*DataManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid storage root: %w", err)
	}

	return &DataManager{
		rootDir: absRoot,
		systemStore:   NewStore[SystemConfig](filepath.Join(absRoot, "config.json")),
		secretStore:   NewStore[SecretData](filepath.Join(absRoot, "secrets.json")),
		registryStore: NewStore[RegistryData](filepath.Join(absRoot, "registry.json")),
	}, nil
}

// LoadAll loads all stores from disk.
func (m *DataManager) LoadAll() error {
	if err := m.systemStore.Load(); err != nil {
		return err
	}
	if err := m.secretStore.Load(); err != nil {
		return err
	}
	if err := m.registryStore.Load(); err != nil {
		return err
	}
	return nil
}

// System returns the system config store.
func (m *DataManager) System() *Store[SystemConfig] { return m.systemStore }

// Secrets returns the secrets store.
func (m *DataManager) Secrets() *Store[SecretData] { return m.secretStore }

// Registry returns the registry store.
func (m *DataManager) Registry() *Store[RegistryData] { return m.registryStore }

// RootDir returns the absolute path to the data directory.
func (m *DataManager) RootDir() string { return m.rootDir }

// WorkspacesDir returns the absolute path to the workspaces directory.
func (m *DataManager) WorkspacesDir() string {
	sys := m.systemStore.Get()
	if sys.WorkspacesDir != "" {
		if filepath.IsAbs(sys.WorkspacesDir) {
			return sys.WorkspacesDir
		}
		return filepath.Join(m.rootDir, sys.WorkspacesDir)
	}
	return filepath.Join(m.rootDir, "workspaces")
}
