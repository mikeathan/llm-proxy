package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

func (s *AppContext) initOrchestrator() {
	dbPath := s.dataMgr.Paths().DatabaseFile()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		logging.Warn("failed to create database directory", "error", err)
		return
	}
	p, err := db.Open(dbPath)
	if err != nil {
		logging.Warn("failed to open orchestrator database", "error", err)
		return
	}
	s.dbProvider = p

	orch, err := orchestrator.NewOrchestrator(p)
	if err != nil {
		logging.Warn("failed to initialize orchestrator, running without budget control", "error", err)
		return
	}
	s.orch = orch
	logging.Info("Orchestrator initialized", "db", dbPath)

	memStore, memErr := memory.New(p)
	if memErr != nil {
		logging.Error("failed to initialize memory store, memory disabled — all memory features unavailable", "error", memErr)
		return
	}
	s.memoryStore = memStore
	logging.Info("Memory store initialized", "db", dbPath)
}

func (s *AppContext) DBProvider() db.Provider {
	return s.dbProvider
}

func (s *AppContext) MemoryStore() *memory.Store {
	return s.memoryStore
}

func (s *AppContext) refreshMetricsService() {
	var ts metrics.TerminalSource
	if s.metrics != nil {
		ts = s.metrics.TerminalSource()
		s.metrics.Stop()
	}
	// Build the metrics service from the FULL live system metrics config.
	// Previously only GPU{...} was passed, silently dropping
	// GPUSampleIntervalSec + GPUSmoothingAlpha: the documented background
	// sampler never started (interval 0 → Start() no-ops) and every metrics
	// snapshot ran an on-demand provider sample (e.g. an ioreg subprocess on
	// macOS) instead of reading the background cache. See
	// docs/PLANS/gpu-performance.md P0.
	sys := s.GetSystem()
	s.metrics = metrics.NewMetricsService(&models.Config{
		Metrics: models.MetricsConfig{
			GPU:                  s.gpuConfig,
			GPUSampleIntervalSec: sys.Metrics.GPUSampleIntervalSec,
			GPUSmoothingAlpha:    sys.Metrics.GPUSmoothingAlpha,
		},
	})
	s.metrics.SetThroughputSource(s.manager)
	if ts != nil {
		s.metrics.SetTerminalSource(ts)
	}
	s.metrics.Start()
}

func (s *AppContext) RefreshMetricsService() {
	s.refreshMetricsService()
}

func (s *AppContext) MetricsSnapshot() metrics.MetricsSnapshot {
	if s.metrics == nil {
		s.refreshMetricsService()
	}
	return s.metrics.Snapshot()
}

func (s *AppContext) SetTerminalSource(src metrics.TerminalSource) {
	if s.metrics != nil {
		s.metrics.SetTerminalSource(src)
	}
}

func (s *AppContext) SetShellProvider(tp shell.ShellProvider) {
	s.terminal = tp
}

func (s *AppContext) ProcessLogger(workspaceID string) logging.Logger {
	if workspaceID == "" {
		return logging.GetGlobalLogger()
	}

	// Cache one FileLogger per workspace: each instance runs a 1-second fsync
	// goroutine and holds an fd until Close, so creating one per call leaked
	// unbounded goroutines/fds on a long-lived service.
	s.procLogMu.Lock()
	defer s.procLogMu.Unlock()
	if l, ok := s.procLoggers[workspaceID]; ok {
		return l
	}

	logFile := s.resolver.ProcessLog(workspaceID)
	l, err := logging.NewFileLogger(logging.Options{
		File:   logFile,
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		return logging.GetGlobalLogger()
	}
	s.procLoggers[workspaceID] = l
	return l
}

func (s *AppContext) ListTemplates() ([]models.TemplateMetadata, error) {
	return s.dataMgr.Templates().List()
}

func (s *AppContext) GetTemplate(id string) (models.Template, error) {
	return s.dataMgr.Templates().Get(id)
}

func (s *AppContext) ResetShell(workspaceID string) error {
	if s.terminal == nil {
		return fmt.Errorf("terminal provider is not initialized")
	}
	s.terminal.Recycle(context.Background(), workspaceID)
	return nil
}

func (s *AppContext) ListShellSessions() []models.TerminalSessionView {
	if s.terminal == nil {
		return nil
	}
	return s.terminal.ListSessions()
}

// FactoryReset runs the staged, failure-safe reset (Phase 10). It best-effort
// quiesces runtime work (stops the watcher) before delegating to the storage
// layer, then re-points the live runtime at the post-reset SecretStore and
// issues exactly one reconciliation (Sync + ApplyModelOverrides) so the
// empty/default post-reset registry/settings take effect without firing
// mid-reset OnChange. The watcher is restarted against the (unchanged)
// ConfigDir/DataDir tethered to the app root context. A full process restart
// is still recommended after reset to rebuild the runtime model catalogue and
// shell sessions against the new state.
func (s *AppContext) FactoryReset() (storage.ResetResult, error) {
	logging.Info("factory reset requested")
	// Refuse to wipe the registry/settings while an agent or automation is
	// executing — resetting under a live run leaves it in an undefined state.
	if s.hasActiveRuns() {
		return storage.ResetResult{}, fmt.Errorf("refusing to factory-reset while assistant/automation runs are active; stop active work first")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := s.dataMgr.Close(closeCtx); err != nil {
		logging.Warn("failed to stop watcher during reset", "error", err)
	}
	res, err := s.dataMgr.FactoryReset()
	if err != nil {
		return res, err
	}
	// The runtime registrar captured the SecretStore at boot; re-point it at
	// the post-reset store (built around the effective key) so live credential
	// resolution keeps working (Phase 10 post-reset secret invariant).
	if s.manager != nil {
		s.manager.SetSecrets(s.dataMgr.Secrets())
		// Exactly one reconciliation: reloadQuiet suppressed OnChange, so push
		// the empty/default post-reset registry+settings into the runtime once.
		s.manager.Sync()
		s.manager.ApplyModelOverrides(s.GetSettings().ModelOverrides)
	}
	// Restart the watcher tethered to the app lifecycle so config watching
	// resumes after the reset swap without leaking an untethered goroutine.
	if s.rootCtx != nil {
		if werr := s.dataMgr.Watch(s.rootCtx); werr != nil {
			logging.Warn("failed to restart watcher after reset", "error", werr)
		}
	}
	return res, nil
}

// ClearRuntimeData deletes only the known allowlist of runtime state (Phase 10).
// It refuses to run while assistant/automation runs or shell sessions are
// active (the .lock/process.log of an active workspace must not be removed),
// and reopens the file logger after logs/ is recreated so subsequent writes do
// not go to a deleted inode.
func (s *AppContext) ClearRuntimeData() error {
	logging.Info("clear runtime data requested")
	if s.hasActiveWork() {
		return fmt.Errorf("refusing to clear runtime data while assistant/automation runs or shell sessions are active; stop active work first")
	}
	if err := s.dataMgr.ClearRuntimeData(); err != nil {
		return err
	}
	// logs/ was removed and recreated; the file logger holds an fd to a deleted
	// inode, so reopen it (Phase 10 file-logger handling).
	if err := logging.ReopenGlobalLogger(); err != nil {
		logging.Warn("failed to reopen file logger after clear-runtime-data", "error", err)
	}
	return nil
}

// Wipeout performs a full uninstall: it refuses while work is active, stops the
// watcher and runtime services (metrics, shell, shared DB), then deletes the
// entire data root plus the workspaces directory. The admin handler owns the
// process exit that follows a successful wipe.
func (s *AppContext) Wipeout() (storage.WipeoutResult, error) {
	logging.Info("wipeout requested")
	if s.hasActiveWork() {
		return storage.WipeoutResult{}, fmt.Errorf("refusing to wipe out while assistant/automation runs or shell sessions are active; stop active work first")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := s.dataMgr.Close(closeCtx); err != nil {
		logging.Warn("failed to stop watcher during wipeout", "error", err)
	}
	// Close metrics/terminal/DB so the deleted files are not held open by the
	// live process (uninstall removes the DB and its -wal/-shm).
	s.Shutdown(closeCtx)
	return s.dataMgr.Wipeout()
}

func (s *AppContext) Shutdown(ctx context.Context) {
	if s.metrics != nil {
		s.metrics.Stop()
	}
	if s.terminal != nil {
		logging.Info("Shutting down shell provider...")
		s.terminal.Shutdown(ctx)
	}
	if s.dbProvider != nil {
		logging.Info("Shutting down shared database...")
		s.dbProvider.DB().Close()
	}
	// Close cached workspace process loggers (stops their fsync goroutines and
	// releases fds).
	s.procLogMu.Lock()
	for ws, l := range s.procLoggers {
		_ = l.Close()
		delete(s.procLoggers, ws)
	}
	s.procLogMu.Unlock()
}
