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
	encSecretStore *Store[models.EncryptedSecretData]
	secretsStore   models.SecretsStore

	// Registry (Models/Providers State)
	registryStore *Store[models.RegistryData]

	// Templates (Reusable task playbooks)
	templateStore *TemplateStore
}


func NewDataManager(rootDir string) (*DataManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid storage root: %w", err)
	}

	mk, err := GetMasterKey()
	if err != nil {
		fmt.Printf("Warning: Failed to load master key: %v\n", err)
	}

	encStore := NewStore[models.EncryptedSecretData](filepath.Join(absRoot, models.SecretsFilename))
	secStore := NewSecretStore(encStore, mk)

	return &DataManager{
		rootDir:        absRoot,
		systemStore:    NewStore[models.SystemConfig](filepath.Join(absRoot, models.SystemConfigFilename)),
		encSecretStore: encStore,
		secretsStore:   secStore,
		registryStore:  NewStore[models.RegistryData](filepath.Join(absRoot, models.RegistryFilename)),
		templateStore:  NewTemplateStore(filepath.Join(absRoot, "templates")),
	}, nil
}

func (m *DataManager) LoadAll() error {
	if err := m.systemStore.Load(); err != nil {
		return err
	}
	if err := m.encSecretStore.Load(); err != nil {
		return err
	}
	if err := m.registryStore.Load(); err != nil {
		return err
	}
	return nil
}

func (m *DataManager) System() *Store[models.SystemConfig] { return m.systemStore }

func (m *DataManager) Secrets() models.SecretsStore { return m.secretsStore }

func (m *DataManager) Registry() *Store[models.RegistryData] { return m.registryStore }
func (m *DataManager) Templates() *TemplateStore { return m.templateStore }

func (m *DataManager) RootDir() string { return m.rootDir }
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
