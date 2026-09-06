package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/paths"
	"llm-proxy/models"
)

// DataManager orchestrates the 3-Tier storage model.
//
// All on-disk locations derive from the resolved paths.Paths — the manager
// never resolves $HOME or env vars itself (Phase 2). The operator+user
// configuration lives in a single AppConfig document (settings.yml) owned by
// appConfigStore; System()/Settings()/HostSettings() are facade views over it so
// the single-owner invariant holds (concurrent system vs settings writes
// serialize through one mutex). All files (settings.yml, registry.json,
// secrets.json, orchestrator.db, templates, etc.) live under a single root via
// typed Paths accessors (Phase 6/7).
type DataManager struct {
	paths paths.Paths

	// appConfigStore owns the merged settings.yml (Tier 1+2 + sandboxing).
	appConfigStore *Store[models.AppConfig]

	// Facade views over appConfigStore.
	systemView   *SystemConfigView
	settingsView *UserSettingsView
	hostView     *hostSettingsView

	// Secrets (Credentials)
	encSecretStore *Store[models.EncryptedSecretData]
	secretsMu      sync.RWMutex // guards secretsStore (rebuilt on factory reset)
	secretsStore   models.SecretsStore

	// Registry (Models/Providers State)
	registryStore *Store[models.RegistryData]

	// Templates (Reusable task playbooks)
	templateStore *TemplateStore

	// Watcher lifecycle: ctx-tethered so the goroutine has an explicit
	// termination path and Close() waits for it to exit (C2/C3). The watcher is
	// stoppable and restartable (needed around factory reset).
	watcherMu   sync.Mutex
	watcher     *fsnotify.Watcher
	watchCancel context.CancelFunc
	watchDone   chan struct{}

	// Per-file debounce timers coalesce bursts of fsnotify events (C3).
	debounceMu sync.Mutex
	debounces  map[string]*time.Timer
}

// NewDataManager constructs the storage layer over the resolved paths. The
// merged settings.yml is the single owner of operator+user configuration. A
// missing or corrupt master key is a fatal error (C4) — the caller must have
// seeded the directory with paths.SeedDefaults first.
func NewDataManager(p paths.Paths) (*DataManager, error) {
	mk, err := GetMasterKey(p.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load master key: %w", err)
	}

	appCfg := NewStore[models.AppConfig](p.ConfigFile())

	encStore := NewStore[models.EncryptedSecretData](p.SecretsFile())
	secStore := NewSecretStore(encStore, mk)

	m := &DataManager{
		paths:          p,
		appConfigStore: appCfg,
		encSecretStore: encStore,
		secretsStore:   secStore,
		registryStore:  NewStore[models.RegistryData](p.RegistryFile()),
		templateStore:  NewTemplateStore(p.TemplatesDir()),
	}

	m.systemView = &SystemConfigView{mgr: m}
	m.settingsView = &UserSettingsView{mgr: m}
	m.hostView = &hostSettingsView{mgr: m}

	return m, nil
}

// watcherDebounce coalesces bursts of fsnotify events per file.
const watcherDebounce = 100 * time.Millisecond

// Watch starts the config-file watcher on the resolved config and data roots,
// tethered to ctx. It is restartable after Close().
func (m *DataManager) Watch(ctx context.Context) error {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	if m.watchDone != nil {
		return fmt.Errorf("watcher already running")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	// ConfigDir and DataDir resolve to the same single root (one-directory
	// layout), so guard against adding the same path twice.
	roots := []string{m.paths.DataDir}
	if m.paths.ConfigDir != m.paths.DataDir {
		roots = append(roots, m.paths.ConfigDir)
	}
	for _, dir := range roots {
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return fmt.Errorf("watch data dir %s: %w", dir, err)
		}
	}

	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.watcher = watcher
	m.watchCancel = cancel
	m.watchDone = done
	m.debounces = make(map[string]*time.Timer)

	go m.watchLoop(watchCtx, watcher, done)
	return nil
}

// watchLoop owns the fsnotify channels. It exits on ctx cancellation or channel
// close, stops pending debounce timers, and closes the watcher before signalling
// done so Close() never races a half-open watcher.
func (m *DataManager) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, done chan struct{}) {
	defer close(done)
	defer func() {
		m.stopDebounces()
		_ = watcher.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			m.handleEvent(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logging.Warn("watcher error", "error", err)
		}
	}
}

// handleEvent debounces events per file: a burst of events for the same path is
// coalesced into a single reload. WriteAtomic temp files are ignored outright.
func (m *DataManager) handleEvent(event fsnotify.Event) {
	filename := filepath.Base(event.Name)
	if strings.HasSuffix(filename, ".tmp") {
		return // WriteAtomic temp file, not a target
	}

	reload := m.reloadFor(filename)
	if reload == nil {
		return
	}

	m.debounceMu.Lock()
	if timer, ok := m.debounces[event.Name]; ok {
		timer.Reset(watcherDebounce)
		m.debounceMu.Unlock()
		return
	}
	timer := time.AfterFunc(watcherDebounce, func() {
		m.debounceMu.Lock()
		delete(m.debounces, event.Name)
		m.debounceMu.Unlock()
		reload()
	})
	m.debounces[event.Name] = timer
	m.debounceMu.Unlock()
}

// reloadFor maps a watched filename to its store reload. Unknown files return nil
// so unrelated events never trigger work.
func (m *DataManager) reloadFor(filename string) func() {
	switch filename {
	case models.SettingsFilename:
		return func() { _ = m.appConfigStore.Load() }
	case models.RegistryFilename:
		return func() { _ = m.registryStore.Load() }
	case models.SecretsFilename:
		return func() { _ = m.encSecretStore.Load() }
	}
	return nil
}

// stopDebounces cancels all pending reload timers.
func (m *DataManager) stopDebounces() {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()
	for name, timer := range m.debounces {
		timer.Stop()
		delete(m.debounces, name)
	}
}

// Close stops the watcher and waits for its goroutine to exit. After Close the
// watcher can be restarted with Watch. Close is safe to call repeatedly. The
// caller's ctx bounds the wait; on deadline Close returns ctx.Err() without
// leaking the watcher goroutine (the watchLoop exits on its own context cancel).
func (m *DataManager) Close(ctx context.Context) error {
	m.watcherMu.Lock()
	cancel := m.watchCancel
	done := m.watchDone
	m.watcherMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			logging.Warn("data manager close timed out waiting for watcher goroutine", "error", ctx.Err())
			return ctx.Err()
		}
	}

	m.watcherMu.Lock()
	m.watchCancel = nil
	m.watchDone = nil
	m.watcher = nil
	m.watcherMu.Unlock()
	return nil
}

// LoadAll loads every store. Missing files are tolerated (SeedDefaults produces a
// valid set on first run); a corrupt file is a real error.
func (m *DataManager) LoadAll() error {
	if err := m.loadAppConfig(); err != nil {
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

// loadAppConfig loads settings.yml and backfills any top-level field absent from
// the file with the canonical DefaultAppConfig(). This prevents a settings.yml
// that predates a newly-added key (e.g. sandboxing) from silently zeroing that
// field — which would otherwise make Sandboxing.Enabled == false and abort
// bootstrap. Persisted values always win over defaults.
func (m *DataManager) loadAppConfig() error {
	if err := m.appConfigStore.Load(); err != nil {
		return err
	}
	loaded := m.appConfigStore.Get()
	def := models.DefaultAppConfig()
	merged := mergeAppConfigDefaults(def, loaded)
	// Only persist when a backfill actually occurred (a key was absent from the
	// file). An already-complete file is left untouched so we never churn disk or
	// fire listeners on a no-op.
	if reflect.DeepEqual(loaded, merged) {
		return nil
	}
	return m.appConfigStore.Update(func(cfg *models.AppConfig) error {
		*cfg = merged
		return nil
	})
}

// mergeAppConfigDefaults overlays d's non-zero top-level fields onto c, leaving
// c's explicit values intact. Pointer fields are merged by value when c's is nil
// but d's is non-nil.
func mergeAppConfigDefaults(d, c models.AppConfig) models.AppConfig {
	if c.Server.Bind == "" {
		c.Server.Bind = d.Server.Bind
	}
	if c.Server.ModelHost == "" {
		c.Server.ModelHost = d.Server.ModelHost
	}
	if c.Server.IdleTimeoutSecs == 0 {
		c.Server.IdleTimeoutSecs = d.Server.IdleTimeoutSecs
	}
	if c.WorkspacesDir == "" {
		c.WorkspacesDir = d.WorkspacesDir
	}
	// Merge Metrics field-by-field so a partially-populated section (e.g. gpu
	// provider cleared but binary/sysfs configured) never clobbers persisted
	// values with the whole default struct.
	if c.Metrics.GPU.Provider == "" {
		c.Metrics.GPU.Provider = d.Metrics.GPU.Provider
	}
	if c.Metrics.GPU.Binary == "" {
		c.Metrics.GPU.Binary = d.Metrics.GPU.Binary
	}
	if c.Metrics.GPU.Index == 0 {
		c.Metrics.GPU.Index = d.Metrics.GPU.Index
	}
	if c.Metrics.GPU.SysfsPath == "" {
		c.Metrics.GPU.SysfsPath = d.Metrics.GPU.SysfsPath
	}
	if c.Metrics.GPUSampleIntervalSec == 0 {
		c.Metrics.GPUSampleIntervalSec = d.Metrics.GPUSampleIntervalSec
	}
	if c.Metrics.GPUSmoothingAlpha == 0 {
		c.Metrics.GPUSmoothingAlpha = d.Metrics.GPUSmoothingAlpha
	}
	if !c.Sandboxing.Enabled && c.Sandboxing == (models.HostSandboxingConfig{}) {
		c.Sandboxing = d.Sandboxing
	}
	if c.Memory == nil {
		c.Memory = d.Memory
	}
	if c.RunLogging == nil {
		c.RunLogging = d.RunLogging
	}
	return c
}

func (m *DataManager) System() *SystemConfigView     { return m.systemView }
func (m *DataManager) Settings() *UserSettingsView   { return m.settingsView }
func (m *DataManager) HostSettings() *hostSettingsView { return m.hostView }

func (m *DataManager) Secrets() models.SecretsStore {
	m.secretsMu.RLock()
	defer m.secretsMu.RUnlock()
	return m.secretsStore
}

func (m *DataManager) EncryptedSecretStore() *Store[models.EncryptedSecretData] {
	return m.encSecretStore
}

func (m *DataManager) Registry() *Store[models.RegistryData] { return m.registryStore }
func (m *DataManager) Templates() *TemplateStore             { return m.templateStore }

// AppConfig returns the merged configuration store. It is exposed for reset
// controls that need to swap or reload the entire document atomically (Phase 10).
func (m *DataManager) AppConfig() *Store[models.AppConfig] { return m.appConfigStore }

// Paths returns the resolved roots backing this manager.
func (m *DataManager) Paths() paths.Paths { return m.paths }

// ConfigDir returns the resolved configuration root.
func (m *DataManager) ConfigDir() string { return m.paths.ConfigDir }

// DataDir returns the resolved data root.
func (m *DataManager) DataDir() string { return m.paths.DataDir }

// RootDir returns the single root directory that holds all config, credentials,
// and runtime state.
func (m *DataManager) RootDir() string { return m.paths.DataDir }

func (m *DataManager) WorkspacesDir() string {
	sys := m.systemView.Get()
	if sys.WorkspacesDir != "" {
		if filepath.IsAbs(sys.WorkspacesDir) {
			return sys.WorkspacesDir
		}
		return filepath.Join(m.paths.DataDir, sys.WorkspacesDir)
	}
	// Default to {repoRoot}/workspaces where {repoRoot} is discovered by walking
	// up from the current working directory to the directory containing
	// backend/go.mod. This anchors the default workspace location to the repo
	// root regardless of which subdirectory the proxy is launched from (dev
	// workflow: `go run main.go` from backend/, frontend/, scripts/, etc.).
	// Place workspace files elsewhere by setting `workspaces_dir` in
	// settings.yml (absolute, or relative to the data root). When no repo root
	// is found (e.g. a packaged/systemd deployment launched from an unrelated
	// cwd) the default falls back to a stable home/data-root location.
	if base := defaultWorkspacesBaseDir(); base != "" {
		return filepath.Join(base, models.WorkspacesDirName)
	}
	// No repo root (packaged/systemd deployment from an unrelated cwd): anchor
	// to the data root — the one guaranteed-writable location. Never fall back
	// to $HOME: under the hardened unit (ProtectHome=read-only,
	// ReadWritePaths=data root) a home path fails with EROFS on first write.
	return filepath.Join(m.paths.DataDir, models.WorkspacesDirName)
}

// defaultWorkspacesBaseDir resolves the base directory for the default
// workspaces location: the repo root (found by walking up from the current
// working directory until a directory containing backend/go.mod is located).
// It returns "" when no repo root is discoverable so callers can fall back to
// a stable home/data-root location.
func defaultWorkspacesBaseDir() string {
	if root, err := findRepoRoot(); err == nil {
		return root
	}
	return ""
}

// findRepoRoot walks up from the current working directory to locate the repo
// root: the nearest ancestor that contains backend/go.mod. Returns the repo
// root directory, or "" with no error when none is found before the filesystem
// root is reached.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		marker := filepath.Join(dir, "backend", "go.mod")
		if _, err := os.Stat(marker); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// MetadataDir returns the per-workspace metadata root (DATA/meta), keeping
// workspace state outside the workspace tree (resolver.go guarantee).
func (m *DataManager) MetadataDir() string {
	return m.paths.MetadataDir()
}
