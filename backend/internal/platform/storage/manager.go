package storage

import (
	"fmt"
	"llm-proxy/models"
	"path/filepath"
)

// DataManager orchestrates the 3-Tier storage model.
type DataManager struct {
	rootDir string

	// Config (System/Infrastructure)
	systemStore *Store[models.SystemConfig]

	// Secrets (Credentials)
	secretStore *Store[models.SecretData]

	// Registry (Models/Providers State)
	registryStore *Store[models.RegistryData]

	// Templates (Reusable task playbooks)
	templateStore *TemplateStore
}


// NewDataManager initializes the storage root and creates the 3-tier stores.
func NewDataManager(rootDir string) (*DataManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid storage root: %w", err)
	}

	return &DataManager{
		rootDir: absRoot,
		systemStore:   NewStore[models.SystemConfig](filepath.Join(absRoot, models.SystemConfigFilename)),
		secretStore:   NewStore[models.SecretData](filepath.Join(absRoot, models.SecretsFilename)),
		registryStore: NewStore[models.RegistryData](filepath.Join(absRoot, models.RegistryFilename)),
		templateStore: NewTemplateStore(filepath.Join(absRoot, "templates")),
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
func (m *DataManager) System() *Store[models.SystemConfig] { return m.systemStore }

// Secrets returns the secrets store.
func (m *DataManager) Secrets() *Store[models.SecretData] { return m.secretStore }

// Registry returns the registry store.
func (m *DataManager) Registry() *Store[models.RegistryData] { return m.registryStore }

// Templates returns the template store.
func (m *DataManager) Templates() *TemplateStore { return m.templateStore }

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
	return filepath.Join(m.rootDir, models.WorkspacesDirName)
}
