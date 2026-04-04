package automation

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

// Dispatcher manages automation execution via a cron scheduler.
type Dispatcher struct {
	registry    *AutomationRegistry
	persistence *persistence.WorkspaceManager
	executor    TaskExecutor
	cron        *cron.Cron
	logger      logging.Logger
	workerCount int

	mu      sync.RWMutex
	jobs    map[string]cron.EntryID // automationID -> cron.EntryID
	stopCh  chan struct{}
	metrics *DispatcherMetrics
}

func NewDispatcher(
	persistence *persistence.WorkspaceManager,
	executor TaskExecutor,
	logger logging.Logger,
	opts ...Option,
) (*Dispatcher, error) {
	d := &Dispatcher{
		registry:    NewAutomationRegistry(),
		persistence: persistence,
		executor:    executor,
		cron: cron.New(cron.WithParser(cron.NewParser(
			cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		))),
		logger:      logger,
		workerCount: 1,
		jobs:        make(map[string]cron.EntryID),
		stopCh:      make(chan struct{}),
		metrics:     &DispatcherMetrics{},
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

type Option func(*Dispatcher)

func WithWorkerCount(n int) Option {
	return func(d *Dispatcher) {
		if n > 0 {
			d.workerCount = n
		}
	}
}

type DispatcherMetrics struct {
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	SkippedExecutions    int64
	TotalLatency         time.Duration
	mu                   sync.Mutex
}

func (m *DispatcherMetrics) RecordExecution(success, skipped bool, latency time.Duration) {
	atomic.AddInt64(&m.TotalExecutions, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalLatency += latency
	if skipped {
		atomic.AddInt64(&m.SkippedExecutions, 1)
	} else if success {
		atomic.AddInt64(&m.SuccessfulExecutions, 1)
	} else {
		atomic.AddInt64(&m.FailedExecutions, 1)
	}
}

func (d *Dispatcher) Start(ctx context.Context) error {
	d.logger.Info("Starting dispatcher")

	workspaces, err := d.persistence.ListWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	for _, ws := range workspaces {
		if err := d.registerWorkspaceAutomations(ws); err != nil {
			d.logger.Error("Failed to register automations for workspace",
				"workspace", ws.ID, "error", err)
		}
	}

	d.cron.Start()

	// Start fsnotify watcher for hot-reload
	watcher, err := d.startWatcher(ctx)
	if err != nil {
		d.logger.Warn("Failed to start fsnotify watcher", "error", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)

	if watcher != nil {
		eg.Go(func() error {
			return d.watchConfigChanges(egCtx, watcher)
		})
	}

	eg.Go(func() error {
		<-egCtx.Done()
		return nil
	})

	<-egCtx.Done()

	if watcher != nil {
		watcher.Close()
	}
	return nil
}

func (d *Dispatcher) Stop() {
	d.logger.Info("Stopping dispatcher")
	close(d.stopCh)
	cronCtx := d.cron.Stop()
	select {
	case <-cronCtx.Done():
		d.logger.Info("All cron jobs finished")
	case <-time.After(30 * time.Second):
		d.logger.Warn("Cron jobs did not finish within timeout")
	}
}

func (d *Dispatcher) Register(workspaceID string, auto *models.Automation) error {
	if err := d.registry.Register(workspaceID, auto); err != nil {
		return err
	}

	entry, ok := d.registry.Get(workspaceID, auto.Name)
	if !ok {
		return fmt.Errorf("automation not registered")
	}

	if entry.Trigger.Type() != "manual" {
		return d.scheduleAutomation(entry)
	}

	return nil
}

func (d *Dispatcher) Unregister(workspaceID, automationName string) error {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return fmt.Errorf("automation not found")
	}

	d.mu.Lock()
	if entryID, exists := d.jobs[entry.ID]; exists {
		d.cron.Remove(entryID)
		delete(d.jobs, entry.ID)
	}
	d.mu.Unlock()

	d.registry.Unregister(workspaceID, automationName)
	return nil
}

func (d *Dispatcher) Trigger(ctx context.Context, workspaceID, automationName string) error {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return fmt.Errorf("automation not found: %s/%s", workspaceID, automationName)
	}

	return d.executeAutomation(ctx, entry)
}

func (d *Dispatcher) ListAll() []*AutomationEntry {
	return d.registry.ListAll()
}

func (d *Dispatcher) Metrics() *DispatcherMetrics {
	return d.metrics
}

func (d *Dispatcher) registerWorkspaceAutomations(ws *models.Workspace) error {
	automations := ws.Config.Automations
	if len(automations) == 0 && ws.Config.CronSchedule != "" {
		automations = []*models.Automation{
			{
				Name:     "default",
				Trigger:  models.TriggerConfig{Type: "cron", Value: ws.Config.CronSchedule},
				TaskFile: "heartbeat.md",
				Strategy: "persistent",
			},
		}
	}

	for _, auto := range automations {
		if err := d.Register(ws.ID, auto); err != nil {
			return fmt.Errorf("failed to register automation %q: %w", auto.Name, err)
		}
	}
	return nil
}

func (d *Dispatcher) scheduleAutomation(entry *AutomationEntry) error {
	if entry.Trigger.Type() == "manual" {
		return nil
	}

	// Determine schedule based on trigger type
	schedule := d.triggerToCron(entry.Trigger)
	if schedule == "" {
		return fmt.Errorf("cannot determine schedule for trigger type %s", entry.Trigger.Type())
	}

	jobFunc := func() {
		// Check ShouldRun to avoid redundant executions
		state, err := d.persistence.ReadState(entry.Workspace)
		if err == nil && !entry.Trigger.ShouldRun(state.NextRunAt, time.Now()) {
			return // Not time to run yet
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := d.executeAutomation(ctx, entry); err != nil {
			d.logger.Error("Automation execution failed",
				"workspace", entry.Workspace,
				"automation", entry.Name,
				"error", err)
		}
	}

	entryID, err := d.cron.AddFunc(schedule, jobFunc)
	if err != nil {
		return fmt.Errorf("failed to add cron function: %w", err)
	}

	d.mu.Lock()
	d.jobs[entry.ID] = entryID
	d.mu.Unlock()

	return nil
}

// triggerToCron converts a Trigger to a cron-compatible schedule string.
func (d *Dispatcher) triggerToCron(tr Trigger) string {
	switch tr.Type() {
	case "cron":
		// CronTrigger itself has the schedule; return the expression
		// Note: This is a simplification; the actual cron is embedded in the trigger
		// We use @every 1m as a poll interval and rely on ShouldRun for actual timing
		return "@every 1m"
	case "interval":
		// Convert interval duration to @every syntax
		// The interval trigger stores duration; we approximate with 1m polling
		return "@every 1m"
	default:
		return ""
	}
}

func (d *Dispatcher) executeAutomation(ctx context.Context, entry *AutomationEntry) error {
	start := time.Now()

	f, err := d.persistence.TryAcquireLock(entry.Workspace)
	if err != nil {
		d.metrics.RecordExecution(false, true, time.Since(start))
		return fmt.Errorf("automation skipped (workspace locked): %w", err)
	}
	defer d.persistence.ReleaseLock(f)

	state, err := d.persistence.ReadState(entry.Workspace)
	if err != nil {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("failed to read state: %w", err)
	}

	taskContent, err := d.persistence.ReadTaskFile(entry.Workspace, entry.TaskFile)
	if err != nil {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("failed to read task file %s: %w", entry.TaskFile, err)
	}
	if taskContent == "" {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("task file %s is empty", entry.TaskFile)
	}

	state.IsRunning = true
	if err := d.persistence.WriteState(entry.Workspace, state); err != nil {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("failed to write state: %w", err)
	}

	execCtx, err := entry.Strategy.Prepare(ctx, entry.Workspace, entry.Name, state)
	if err != nil {
		state.IsRunning = false
		d.persistence.WriteState(entry.Workspace, state)
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("strategy preparation failed: %w", err)
	}

	req := ExecuteRequest{
		WorkspaceID:    entry.Workspace,
		AutomationName: entry.Name,
		TaskFile:       entry.TaskFile,
		Strategy:       entry.Strategy,
		State:          state,
	}

	resp, err := d.executor.Execute(execCtx, req)
	elapsed := time.Since(start)

	if err != nil {
		state.IsRunning = false
		state.LastError = err.Error()
		d.persistence.WriteState(entry.Workspace, state)
		d.metrics.RecordExecution(false, false, elapsed)
		return err
	}

	if resp != nil && resp.State != nil {
		if resp.Output != "" && resp.State != nil {
			ApplyPulseLogic(resp)
		}

		state.IsRunning = false
		if resp.Output != "" {
			state.LastOutput = resp.Output
			state.LastError = ""
		}
		if !resp.State.LastPulse.IsZero() {
			state.LastPulse = resp.State.LastPulse
		}
		d.persistence.WriteState(entry.Workspace, state)
	}

	d.metrics.RecordExecution(true, false, elapsed)
	return nil
}

// ============================================================================
// fsnotify Hot-Reload
// ============================================================================
func (d *Dispatcher) startWatcher(ctx context.Context) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	if err := watcher.Add(d.persistence.BaseDir()); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch base directory: %w", err)
	}
	d.logger.Info("Started fsnotify watcher", "path", d.persistence.BaseDir())
	return watcher, nil
}

func (d *Dispatcher) watchConfigChanges(ctx context.Context, watcher *fsnotify.Watcher) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only react to config.yaml changes
			if filepath.Base(event.Name) == "config.yaml" {
				d.logger.Info("Config change detected, reconciling automations", "file", event.Name)
				d.handleConfigChange()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			d.logger.Error("Watcher error", "error", err)
		}
	}
}

func (d *Dispatcher) handleConfigChange() {
	// Re-list workspaces and reconcile automations
	workspaces, err := d.persistence.ListWorkspaces()
	if err != nil {
		d.logger.Error("Failed to list workspaces during config change", "error", err)
		return
	}

	activeIDs := make(map[string]bool)
	for _, ws := range workspaces {
		activeIDs[ws.ID] = true

		// Unregister old automations for this workspace
		d.registry.UnregisterWorkspace(ws.ID)

		// Remove old cron jobs
		d.mu.Lock()
		for id, entryID := range d.jobs {
			if hasPrefix(id, ws.ID+"/") {
				d.cron.Remove(entryID)
				delete(d.jobs, id)
			}
		}
		d.mu.Unlock()

		// Re-register automations
		if err := d.registerWorkspaceAutomations(ws); err != nil {
			d.logger.Error("Failed to re-register automations", "workspace", ws.ID, "error", err)
		}
	}

	// Remove deleted workspaces
	d.mu.Lock()
	for id := range d.jobs {
		parts := splitID(id)
		if len(parts) == 2 && !activeIDs[parts[0]] {
			d.cron.Remove(d.jobs[id])
			delete(d.jobs, id)
			d.logger.Info("Removed deleted workspace from dispatcher", "workspace", parts[0])
		}
	}
	d.mu.Unlock()
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func splitID(id string) []string {
	for i := 0; i < len(id); i++ {
		if id[i] == '/' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

// Persistence returns the underlying WorkspaceManager.
func (d *Dispatcher) Persistence() *persistence.WorkspaceManager {
	return d.persistence
}
