package automation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/safe"
	"llm-proxy/models"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

const MaxHistorySize = 100

// defaultAutomationTimeout is the wall-clock cap for a single automation run
// when the automation pins no model with a longer timeout_minutes. The
// effective per-run bound is max(defaultAutomationTimeout, model timeout), so
// a slow model configured with timeout_minutes > 10m is not cut off mid-run.
const defaultAutomationTimeout = 10 * time.Minute

// shellPGIDPollInterval is the polling frequency for discovering the shell
// process group ID after an automation starts. The shell session is created
// lazily on first use, so we poll until it's available or the context expires.
const shellPGIDPollInterval = 2 * time.Second

// stopDiagnosticDelay is the time StopAutomation waits before checking
// whether a run has actually terminated. It is a per-Dispatcher field
// (defaultDiagnosticDelay) so tests can set it to zero on their own instance to
// exercise the force-kill path synchronously without mutating a shared global.
const defaultDiagnosticDelay = 30 * time.Second

// ErrNoActiveRun is returned by StopAutomation when the workspace has no
// in-flight automation to stop. Callers that stop-then-cleanup (e.g. workspace
// deletion) can treat this as benign.
var ErrNoActiveRun = errors.New("no active automation found")

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

	historyMu     sync.RWMutex
	globalHistory []models.AutomationRun
	events        *EventBus

	runMu      sync.RWMutex
	activeRuns map[string]*activeRun // workspaceID -> run metadata
	// diagnosticDelay is how long StopAutomation waits before force-killing an
	// unresponsive shell. Zero means the default (defaultDiagnosticDelay).
	diagnosticDelay time.Duration
	// automationTimeout is the base wall-clock cap for a single automation run
	// (overridable via WithAutomationTimeout); the effective bound is max(this,
	// the pinned model's timeout_minutes).
	automationTimeout time.Duration

	stopOnce sync.Once
}

// activeRun tracks a running automation session including its cancellation
// function and optional shell process group ID for force-termination.
type activeRun struct {
	cancel     context.CancelFunc
	pgid       int                // negated PGID for syscall.Kill; 0 when no active shell
	diagCancel context.CancelFunc // cancels the StopAutomation diagnostic goroutine
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
		// cron.Recover wraps every scheduled job so a panic inside a run can
		// never crash the whole service (robfig/cron runs jobs in bare
		// goroutines with no recovery of its own).
		cron: cron.New(
			cron.WithParser(cron.NewParser(
				cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
			)),
			cron.WithChain(cron.Recover(cronLogger{logger})),
		),
		logger:          logger,
		workerCount:     1,
		jobs:            make(map[string]cron.EntryID),
		stopCh:          make(chan struct{}),
		metrics:         &DispatcherMetrics{},
		events:          NewEventBus(),
		activeRuns:      make(map[string]*activeRun),
		diagnosticDelay: defaultDiagnosticDelay,
		automationTimeout: defaultAutomationTimeout,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

// cronLogger adapts the app logger to robfig/cron's Logger interface so
// cron.Recover can report panicked jobs through the normal logging pipeline.
type cronLogger struct{ l logging.Logger }

func (c cronLogger) Info(msg string, kv ...any)  { c.l.Info(msg, kv...) }
func (c cronLogger) Error(err error, msg string, kv ...any) {
	c.l.Error(msg, append([]any{"error", err}, kv...)...)
}

type Option func(*Dispatcher)

func WithWorkerCount(n int) Option {
	return func(d *Dispatcher) {
		if n > 0 {
			d.workerCount = n
		}
	}
}

// WithAutomationTimeout sets the base wall-clock cap for a single automation
// run. The effective bound is max(this, the pinned model's timeout_minutes),
// so a slow model configured with a longer timeout is not cut off mid-run.
func WithAutomationTimeout(t time.Duration) Option {
	return func(d *Dispatcher) {
		if t > 0 {
			d.automationTimeout = t
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

	// Load historical runs from all workspaces to populate global ledger
	d.LoadHistory()

	workspaces, err := d.persistence.ListWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	for _, ws := range workspaces {
		// Cleanup stale execution state from previous runs
		if ws.State.IsRunning() {
			d.logger.Warn("Stale execution state detected on startup, resetting", "workspace", ws.ID)
			ws.State.SetRunning("")
			if err := d.persistence.WriteState(ws.ID, &ws.State); err != nil {
				d.logger.Error("Failed to reset stale state", "workspace", ws.ID, "error", err)
			}
		}

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

func (d *Dispatcher) Stop(ctx context.Context) {
	d.stopOnce.Do(func() {
		d.logger.Info("Stopping dispatcher")
		close(d.stopCh)
		if d.events != nil {
			d.events.Stop()
		}
		cronCtx := d.cron.Stop()
		select {
		case <-cronCtx.Done():
			d.logger.Info("All cron jobs finished")
		case <-ctx.Done():
			d.logger.Warn("Cron jobs did not finish within shutdown deadline")
		}
	})
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

// UnregisterWorkspace removes all scheduled automations for a given workspace.
func (d *Dispatcher) UnregisterWorkspace(workspaceID string) {
	d.registry.UnregisterWorkspace(workspaceID)

	d.mu.Lock()
	for id, entryID := range d.jobs {
		if hasPrefix(id, workspaceID+"/") {
			d.cron.Remove(entryID)
			delete(d.jobs, id)
		}
	}
	d.mu.Unlock()
}

func (d *Dispatcher) Trigger(ctx context.Context, workspaceID, automationName, recordingRef string) error {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return fmt.Errorf("automation not found: %s/%s", workspaceID, automationName)
	}

	// The context is cancellable; the run timeout (max of the dispatcher cap
	// and the pinned model's timeout_minutes) is applied in executeAutomation.
	triggerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	return d.executeAutomation(triggerCtx, entry, recordingRef)
}

func (d *Dispatcher) Events() *EventBus {
	return d.events
}

func (d *Dispatcher) ListAll() []*AutomationEntry {
	return d.registry.ListAll()
}

func (d *Dispatcher) Metrics() *DispatcherMetrics {
	return d.metrics
}

// GlobalActivity returns the rolling global ledger of recent events.
func (d *Dispatcher) GlobalActivity() []models.AutomationRun {
	d.historyMu.RLock()
	defer d.historyMu.RUnlock()

	// Return a copy to avoid data races
	res := make([]models.AutomationRun, len(d.globalHistory))
	copy(res, d.globalHistory)
	return res
}

func (d *Dispatcher) RecordActivity(run models.AutomationRun) {
	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	d.globalHistory = append(d.globalHistory, run)
	if len(d.globalHistory) > MaxHistorySize {
		d.globalHistory = d.globalHistory[len(d.globalHistory)-MaxHistorySize:]
	}
}

// ClearWorkspaceHistory purges all runs for a specific workspace from the global history.
func (d *Dispatcher) ClearWorkspaceHistory(workspaceID string) {
	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	var newHistory []models.AutomationRun
	for _, run := range d.globalHistory {
		if run.WorkspaceID != workspaceID {
			newHistory = append(newHistory, run)
		}
	}
	d.globalHistory = newHistory
}

// LoadHistory populates the global history from persistent workspace states.
func (d *Dispatcher) LoadHistory() {
	workspaces, err := d.persistence.ListWorkspaces()
	if err != nil {
		d.logger.Error("Failed to list workspaces for history load", "error", err)
		return
	}

	var allRuns []models.AutomationRun
	for _, ws := range workspaces {
		state, err := d.persistence.ReadState(ws.ID)
		if err == nil {
			// Ensure WorkspaceID is set even for legacy records
			for i := range state.History {
				if state.History[i].WorkspaceID == "" {
					state.History[i].WorkspaceID = ws.ID
				}
			}
			allRuns = append(allRuns, state.History...)
		}
	}

	// Sort chronologically (oldest to newest)
	sort.Slice(allRuns, func(i, j int) bool {
		return allRuns[i].Timestamp.Before(allRuns[j].Timestamp)
	})

	d.historyMu.Lock()
	defer d.historyMu.Unlock()

	if len(allRuns) > MaxHistorySize {
		allRuns = allRuns[len(allRuns)-MaxHistorySize:]
	}
	d.globalHistory = allRuns

	// Reset and recalculate metrics from history
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()
	d.metrics.TotalExecutions = 0
	d.metrics.SuccessfulExecutions = 0
	d.metrics.FailedExecutions = 0
	d.metrics.SkippedExecutions = 0
	d.metrics.TotalLatency = 0

	for _, run := range allRuns {
		d.metrics.TotalExecutions++
		d.metrics.TotalLatency += time.Duration(run.DurationMs) * time.Millisecond
		if run.Error == "" {
			d.metrics.SuccessfulExecutions++
		} else {
			d.metrics.FailedExecutions++
		}
	}

	d.logger.Info("Loaded global history", "count", len(d.globalHistory), "total_executions", d.metrics.TotalExecutions)
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
		if err != nil {
			return
		}

		// Use per-automation last run time if available
		var lastRun time.Time
		if last, ok := state.LastRuns[entry.Name]; ok {
			lastRun = last.Timestamp
		}

		if !entry.Trigger.ShouldRun(lastRun, time.Now()) {
			return // Not time to run yet
		}

		// The run context is cancellable; the run timeout (max of the
		// dispatcher cap and the pinned model's timeout_minutes) is applied in
		// executeAutomation.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := d.executeAutomation(ctx, entry, ""); err != nil {
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

// triggerToCron converts a Trigger to a cron-compatible schedule string. The
// real expression is registered so jobs fire on their actual schedule — the
// previous @every-1m poll made a pre-executor failure retry every minute and
// fired a brand-new automation immediately regardless of its schedule.
func (d *Dispatcher) triggerToCron(tr Trigger) string {
	switch tr.Type() {
	case "cron":
		return tr.Value()
	case "interval":
		return "@every " + tr.Value()
	default:
		return ""
	}
}

// runContext derives the per-run execution context. The run is cancellable and
// bounded by max(automationTimeout, the pinned model's timeout_minutes), so a
// model configured with timeout_minutes > the dispatcher cap is not cut off
// mid-run while a run without a model override stays within the dispatcher cap.
func (d *Dispatcher) runContext(ctx context.Context, entry *AutomationEntry) (context.Context, context.CancelFunc) {
	timeout := d.automationTimeout
	if entry.Model != "" {
		if mt := d.executor.ModelTimeout(entry.Model); mt > timeout {
			timeout = mt
		}
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (d *Dispatcher) executeAutomation(ctx context.Context, entry *AutomationEntry, recordingRefOverride string) (retErr error) {
	start := time.Now()

	// Contain panics so a single bad run can never crash the service. This
	// also clears the run's "running" state and records the failure so the
	// workspace is not left marked running after a panic.
	defer func() {
		if rec := recover(); rec != nil {
			d.logger.Error("panic in automation run",
				"workspace", entry.Workspace, "automation", entry.Name,
				"error", fmt.Sprintf("%v", rec), "stack", string(debug.Stack()))
			if state, sErr := d.persistence.ReadState(entry.Workspace); sErr == nil {
				state.SetRunning("")
				_ = d.persistence.WriteState(entry.Workspace, state)
			}
			d.metrics.RecordExecution(false, false, time.Since(start))
			retErr = fmt.Errorf("automation panicked: %v", rec)
		}
	}()

	// 0. Setup Cancellation — the run is bounded by max(automationTimeout, the
	// pinned model's timeout_minutes), so a slow model configured with a longer
	// timeout is not cut off by the dispatcher cap.
	execCtx, cancel := d.runContext(ctx, entry)
	defer cancel()

	f, err := d.persistence.TryAcquireLock(entry.Workspace)
	if err != nil {
		d.metrics.RecordExecution(false, true, time.Since(start))
		return fmt.Errorf("automation skipped (workspace locked): %w", err)
	}
	defer d.persistence.ReleaseLock(f)

	d.runMu.Lock()
	d.activeRuns[entry.Workspace] = &activeRun{cancel: cancel}
	d.runMu.Unlock()
	// Poll for the shell PGID after the agent starts; the persistent shell
	// session is created lazily on first terminal_execute call.
	safe.Go("shell PGID poll", func() { d.pollShellPGID(execCtx, entry.Workspace) })
	defer func() {
		d.runMu.Lock()
		delete(d.activeRuns, entry.Workspace)
		d.runMu.Unlock()
	}()

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

	state.SetRunning(entry.Name)
	if err := d.persistence.WriteState(entry.Workspace, state); err != nil {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Immediately notify UI that execution has actively started
	d.events.Clear(entry.Workspace, assistant.ChannelAutomation)
	d.events.Publish(entry.Workspace, assistant.AgentEvent{
		Type:      assistant.EventMessage,
		Channel:   assistant.ChannelAutomation,
		Timestamp: time.Now(),
		Payload: proxy.Message{
			Role:    "system",
			Content: fmt.Sprintf("▶ Booting automation: %s\nLoading task file: %s", entry.Name, entry.TaskFile),
		},
	})

	stratCtx, err := entry.Strategy.Prepare(execCtx, entry.Workspace, entry.Name, state)
	if err != nil {
		state.SetRunning("")
		d.persistence.WriteState(entry.Workspace, state)
		d.metrics.RecordExecution(false, false, time.Since(start))
		return fmt.Errorf("strategy preparation failed: %w", err)
	}

	recordingRef := entry.RecordingRef
	if recordingRefOverride != "" {
		recordingRef = recordingRefOverride
	}
	req := ExecuteRequest{
		WorkspaceID:    entry.Workspace,
		AutomationName: entry.Name,
		TaskFile:       entry.TaskFile,
		TaskContent:    taskContent,
		Strategy:       entry.Strategy,
		State:          state,
		Model:          entry.Model,
		LoopStrategy:   string(entry.LoopStrategy),
		AllowedTools:   entry.AllowedTools,
		RecordingRef:   recordingRef,
	}

	resp, err := d.executor.Execute(stratCtx, req)
	elapsed := time.Since(start)

	if err != nil {
		state.SetRunning("")
		d.persistence.WriteState(entry.Workspace, state)

		// Un-hang the UI by publishing the error over the EventBus
		d.events.Publish(entry.Workspace, assistant.AgentEvent{
			Type:    assistant.EventError,
			Channel: assistant.ChannelAutomation,
			Payload: proxy.Message{
				Role:    "system",
				Content: fmt.Sprintf("Execution Error: %v", err),
			},
		})

		// Ensure failed runs also propagate to the global ledger
		if len(state.History) > 0 {
			d.RecordActivity(state.History[len(state.History)-1])
		}

		d.metrics.RecordExecution(false, false, elapsed)
		return err
	}

	if resp != nil && resp.State != nil {
		if resp.Output != "" && resp.State != nil {
			ApplyPulseLogic(resp)
		}

		state.SetRunning("")
		if !resp.State.LastPulse.IsZero() {
			state.LastPulse = resp.State.LastPulse
		}
		d.persistence.WriteState(entry.Workspace, state)

		// Also record in global history
		if len(state.History) > 0 {
			d.RecordActivity(state.History[len(state.History)-1])
		}
	} else if err != nil {
		// Recorded in error path above
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

func (d *Dispatcher) StopAutomation(workspaceID string) error {
	d.runMu.Lock()
	r, ok := d.activeRuns[workspaceID]
	d.runMu.Unlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrNoActiveRun, workspaceID)
	}

	r.cancel()
	d.logger.Info("Automation stopped by user request", "workspace", workspaceID)

	// Cancel any previous diagnostic goroutine for this workspace to
	// prevent accumulation on repeated StopAutomation calls.
	if r.diagCancel != nil {
		r.diagCancel()
	}
	diagCtx, diagCancel := context.WithCancel(context.Background())
	r.diagCancel = diagCancel

	safe.Go("automation stop diagnostic", func() {
		delay := d.diagnosticDelay
		if delay <= 0 {
			delay = defaultDiagnosticDelay
		}
		select {
		case <-time.After(delay):
			d.runMu.Lock()
			currentRun, stillRunning := d.activeRuns[workspaceID]
			isSameRun := stillRunning && currentRun == r
			pgid := 0
			if isSameRun && r.pgid != 0 {
				pgid = r.pgid
			}
			d.runMu.Unlock()

			if pgid != 0 {
				d.logger.Error("automation stop: force-killing shell process group",
					"workspace", workspaceID, "pgid", pgid)
				_ = syscall.Kill(pgid, syscall.SIGKILL)
				d.runMu.Lock()
				if d.activeRuns[workspaceID] == r {
					delete(d.activeRuns, workspaceID)
				}
				d.runMu.Unlock()
			} else if isSameRun {
				d.logger.Warn("automation stop: cancellation did not terminate the run within 30s (no shell PGID available)",
					"workspace", workspaceID)
			}
			// else: old run already finished and replaced by a new run —
			// silent no-op, no stale PGID used.
		case <-diagCtx.Done():
			// Previous diagnostic goroutine was cancelled by a subsequent
			// StopAutomation call — exit cleanly.
		}
	})

	return nil
}

// pollShellPGID periodically queries the task executor for the shell PGID
// and stores it on the activeRun. It exits when the context is cancelled
// (the automation finishes) or when a valid PGID is found.
func (d *Dispatcher) pollShellPGID(ctx context.Context, workspaceID string) {
	ticker := time.NewTicker(shellPGIDPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pgid, err := d.executor.ShellPGID(context.Background(), workspaceID)
			if err != nil || pgid == 0 {
				continue
			}
			d.runMu.Lock()
			if r, ok := d.activeRuns[workspaceID]; ok {
				r.pgid = pgid
				d.runMu.Unlock()
				return
			}
			d.runMu.Unlock()
			return
		}
	}
}

// IsAutomationRunning reports whether an automation is currently executing
// in the given workspace. It is a read-only check safe for use in HTTP
// handlers and observability — it never cancels or mutates the run.
func (d *Dispatcher) IsAutomationRunning(workspaceID string) bool {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	_, ok := d.activeRuns[workspaceID]
	return ok
}

// Persistence returns the underlying WorkspaceManager.
func (d *Dispatcher) Persistence() *persistence.WorkspaceManager {
	return d.persistence
}
