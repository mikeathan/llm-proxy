package app

import (
	"context"
	"sync"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

type AppContext struct {
	manager       llm.RuntimeManager
	orch          *orchestrator.Orchestrator
	dbProvider    db.Provider   // shared SQLite connection for ledger + memory
	memoryStore   *memory.Store // agent memory, nil when disabled
	dataMgr       *storage.DataManager
	resolver      *storage.PathResolver
	rootDir       string
	gpuConfig     models.GPUConfig
	metrics       *metrics.MetricsService
	terminal      shell.ShellProvider
	configMu      sync.RWMutex
	cliEnableRuns bool

	// rootCtx is the application lifecycle context, injected by app.New so the
	// watcher restarted after factory reset stays tethered to a cancellable
	// root (Constitution II.2/II.14) instead of a leaked context.Background().
	rootCtx context.Context

	// activeWork reports whether any assistant run or automation is executing.
	// Registered at assembly time (buildHTTP); a nil checker is treated as
	// "no known active work" for shell-session-only guards.
	activeWork func() bool

	// procLogMu guards procLoggers: one cached FileLogger per workspace so
	// process logs don't leak an fd + fsync goroutine per call (each
	// FileLogger runs a 1s syncLoop until closed).
	procLogMu   sync.Mutex
	procLoggers map[string]*logging.FileLogger
}

func NewServer(mgr llm.RuntimeManager, dataMgr *storage.DataManager) *AppContext {
	logging.Info("Initializing AppContext server state")

	sys := dataMgr.System().Get()
	rootDir := dataMgr.RootDir()

	s := &AppContext{
		manager:     mgr,
		dataMgr:     dataMgr,
		resolver:    storage.NewPathResolver(rootDir, dataMgr.WorkspacesDir(), dataMgr.MetadataDir()),
		rootDir:     rootDir,
		gpuConfig:   sys.Metrics.GPU,
		procLoggers: make(map[string]*logging.FileLogger),
	}

	s.initOrchestrator()

	// Link manager to secrets
	if m, ok := mgr.(*llm.LLMRuntimeManager); ok {
		m.SetSecrets(dataMgr.Secrets())
	}

	logging.Debug("State initialized",
		"root", rootDir,
		"workspaces", s.resolver.WorkspacesRoot())

	s.refreshMetricsService()

	s.registerSubscribers()

	return s
}

func (s *AppContext) registerSubscribers() {
	// 1. Settings Changes -> Sync LLM Runtime & Local Paths
	s.dataMgr.Settings().OnChange(func(u models.UserSettings) {
		logging.Info("Settings change detected, syncing LLM runtime", "modelDir", u.Local.ModelDir)
		if reg := s.manager.Registrar(); reg != nil {
			reg.RegisterLocal(u.Local.LlamaServerBinary, u.Local.ModelDir, u.Local.DefaultArgs)
		}
		s.manager.Sync()
		logging.Info("Settings change: Sync completed, applying model overrides", "override_count", len(u.ModelOverrides))
		s.manager.ApplyModelOverrides(u.ModelOverrides)
		logging.Info("Settings change: model overrides applied")
	})

	// 2. System Config Changes -> Sync Infrastructure & Environment
	s.dataMgr.System().OnChange(func(sys models.SystemConfig) {
		logging.Info("System config change detected, syncing infrastructure")
		s.SetGPUConfig(sys.Metrics.GPU)
		s.manager.SetModelHost(sys.Server.ModelHost)

		// Push environment updates to active models
		for _, m := range s.manager.ListModels() {
			m.Environment = sys.Server.Environment
			_ = s.manager.UpdateModel(m)
		}
	})

	// 3. Registry Changes -> Sync LLM Runtime (Providers, Catalogue)
	s.dataMgr.Registry().OnChange(func(reg models.RegistryData) {
		logging.Info("Registry change detected, syncing LLM runtime")
		s.manager.Sync()
		s.manager.ApplyModelOverrides(s.GetSettings().ModelOverrides)
	})

	// 4. Secrets Changes -> Sync LLM Runtime (credentials updated)
	s.dataMgr.EncryptedSecretStore().OnChange(func(data models.EncryptedSecretData) {
		logging.Info("Secrets change detected, syncing LLM runtime")
		s.manager.Sync()
		s.manager.ApplyModelOverrides(s.GetSettings().ModelOverrides)
	})
}

func (a *AppContext) SelectModels() (string, string) {
	reg := a.dataMgr.Registry().Get()
	p := reg.PrimaryModel
	f := reg.FallbackModel

	// If no primary is set, fall back to the fallback model only — never an
	// arbitrary catalogue entry. Callers already error on an empty primary.
	if p == "" {
		p = f
	}

	return p, f
}

func (s *AppContext) Runtime() llm.RuntimeManager {
	return s.manager
}

func (s *AppContext) Orchestrator() *orchestrator.Orchestrator {
	return s.orch
}

func (s *AppContext) Manager() llm.RuntimeManager {
	return s.manager
}

func (s *AppContext) GetSettings() models.UserSettings {
	return s.dataMgr.Settings().Get()
}

func (s *AppContext) UpdateSettings(fn func(*models.UserSettings)) error {
	return s.dataMgr.Settings().Update(func(set *models.UserSettings) error {
		fn(set)
		return nil
	})
}

func (s *AppContext) Secrets() models.SecretsStore {
	return s.dataMgr.Secrets()
}

func (s *AppContext) RootDir() string {
	return s.resolver.RootDir()
}

func (s *AppContext) Resolver() storage.Resolver {
	return s.resolver
}

func (s *AppContext) WorkspacesDir() string {
	return s.resolver.WorkspacesRoot()
}

func (s *AppContext) MetadataDir() string {
	return s.resolver.MetadataRoot()
}

func (s *AppContext) SetModelDir(dir string) {
	_ = s.UpdateSettings(func(set *models.UserSettings) {
		set.Local.ModelDir = dir
	})
}

func (s *AppContext) SetWorkspacesDir(dir string) {
	s.resolver = storage.NewPathResolver(s.resolver.RootDir(), dir, s.dataMgr.MetadataDir())
}

func (s *AppContext) SetRootContext(ctx context.Context) {
	s.rootCtx = ctx
}

func (s *AppContext) SetActiveWorkChecker(fn func() bool) {
	s.activeWork = fn
}

// hasActiveRuns reports whether any assistant or automation run is executing.
// Used by factory-reset: the reset wipes the registry under a live agent, so it
// must not proceed while a run is active. Shell sessions are intentionally NOT
// part of this check — factory-reset leaves workspaces/meta/shell state intact.
func (s *AppContext) hasActiveRuns() bool {
	return s.activeWork != nil && s.activeWork()
}

// hasActiveWork reports whether clear-runtime-data would clobber live state:
// any registered assistant/automation run, or any live persistent shell session.
func (s *AppContext) hasActiveWork() bool {
	if s.hasActiveRuns() {
		return true
	}
	if s.terminal != nil && len(s.terminal.ListSessions()) > 0 {
		return true
	}
	return false
}
