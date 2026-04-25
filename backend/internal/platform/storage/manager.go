package storage

import (
	"fmt"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DataManager orchestrates the 3-Tier storage model.
type DataManager struct {
	rootDir string

	// Config (System/Infrastructure)
	systemStore *Store[models.SystemConfig]

	// Settings (User Preferences)
	settingsStore *Store[models.UserSettings]

	// Secrets (Credentials)
	encSecretStore *Store[models.EncryptedSecretData]
	secretsStore   models.SecretsStore

	// Registry (Models/Providers State)
	registryStore *Store[models.RegistryData]

	// Templates (Reusable task playbooks)
	templateStore *TemplateStore

	watcher *fsnotify.Watcher
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
		settingsStore:  NewStore[models.UserSettings](filepath.Join(getMetadataDir(absRoot), models.SettingsFilename)),
		encSecretStore: encStore,
		secretsStore:   secStore,
		registryStore:  NewStore[models.RegistryData](filepath.Join(absRoot, models.RegistryFilename)),
		templateStore:  NewTemplateStore(filepath.Join(absRoot, "templates")),
	}, nil
}

func (m *DataManager) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	m.watcher = watcher

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// debounce slightly to avoid multiple reloads on rapid writes
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Remove == fsnotify.Remove {
					time.Sleep(100 * time.Millisecond)
					filename := filepath.Base(event.Name)
					switch filename {
					case models.SystemConfigFilename:
						_ = m.systemStore.Load()
					case models.SettingsFilename:
						_ = m.settingsStore.Load()
					case models.RegistryFilename:
						_ = m.registryStore.Load()
					case models.SecretsFilename:
						_ = m.encSecretStore.Load()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("Watcher error: %v\n", err)
			}
		}
	}()

	// Watch root and metadata dirs
	if err := watcher.Add(m.rootDir); err != nil {
		return err
	}
	metadataDir := m.MetadataDir()
	if _, err := os.Stat(metadataDir); err == nil {
		if err := watcher.Add(metadataDir); err != nil {
			return err
		}
	}

	return nil
}

func (m *DataManager) Close() error {
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}

func (m *DataManager) LoadAll() error {
	if err := m.systemStore.Load(); err != nil {
		return err
	}
	if err := m.settingsStore.Load(); err != nil {
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

func (m *DataManager) Settings() *Store[models.UserSettings] { return m.settingsStore }

func (m *DataManager) Secrets() models.SecretsStore { return m.secretsStore }

func (m *DataManager) Registry() *Store[models.RegistryData] { return m.registryStore }
func (m *DataManager) Templates() *TemplateStore             { return m.templateStore }

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

func (m *DataManager) MetadataDir() string {
	return getMetadataDir(m.rootDir)
}

func getMetadataDir(rootDir string) string {
	// Priority 1: Explicit override via env var (recommended for systemd deployments).
	if envDir := os.Getenv("LLM_PROXY_CONFIG_DIR"); envDir != "" {
		return envDir
	}

	// Priority 2: XDG/home-based config dir — but only if it is actually writable.
	// Under systemd with ProtectHome= or ReadOnlyPaths=, os.UserHomeDir() may
	// resolve fine yet the path is on a read-only filesystem, causing write failures.
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".config", "llm-proxy")
		if isWritableDir(candidate) {
			return candidate
		}
	}

	// Priority 3: Fallback to a hidden folder inside the app data root.
	// This is always writable because it lives next to the rest of the app data.
	return filepath.Join(rootDir, models.InternalDirName)
}

// isWritableDir returns true if dir exists and a temp file can be created inside it.
// If the directory does not exist yet, we attempt to create it — if that succeeds
// the location is writable.
func isWritableDir(dir string) bool {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".write-probe-*.tmp")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return true
}

