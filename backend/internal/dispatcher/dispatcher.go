package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/persistence"
	"llm-proxy/models"

	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

// Dispatcher manages automation execution via a cron scheduler.
type Dispatcher struct {
	registry   *AutomationRegistry
	persistence *persistence.WorkspaceManager
	executor   TaskExecutor
	cron       *cron.Cron
	logger     *slog.Logger
	workerCount int

	mu        sync.RWMutex
	jobs      map[string]cron.EntryID // automationID -> cron.EntryID
	stopCh    chan struct{}
	metrics   *DispatcherMetrics
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(
	persistence *persistence.WorkspaceManager,
	executor TaskExecutor,
	logger *slog.Logger,
	opts ...Option,
) (*Dispatcher, error) {
	d := &Dispatcher{
		registry:   NewAutomationRegistry(),
		persistence: persistence,
		executor:   executor,
		cron: cron.New(cron.WithParser(cron.NewParser(
			cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		))),
		logger:     logger,
		workerCount: 1,
		jobs:       make(map[string]cron.EntryID),
		stopCh:    make(chan struct{}),
		metrics:   &DispatcherMetrics{},
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

// Option configures a Dispatcher.
type Option func(*Dispatcher)

func WithWorkerCount(n int) Option {
	return func(d *Dispatcher) {
		if n > 0 {
			d.workerCount = n
		}
	}
}

// DispatcherMetrics holds runtime metrics for the dispatcher.
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

// Start initializes the dispatcher: loads workspaces and starts the cron scheduler.
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

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		<-egCtx.Done()
		return nil
	})

	<-egCtx.Done()
	return nil
}

// Stop gracefully shuts down the dispatcher.
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

// Register adds an automation to the dispatcher. If trigger has a schedule, it is added to cron.
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

// Unregister removes an automation from the dispatcher.
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

// Trigger manually triggers an automation, bypassing ShouldRun.
func (d *Dispatcher) Trigger(ctx context.Context, workspaceID, automationName string) error {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return fmt.Errorf("automation not found: %s/%s", workspaceID, automationName)
	}

	return d.executeAutomation(ctx, entry)
}

// ============================================================================
// Internal
// ============================================================================

func (d *Dispatcher) registerWorkspaceAutomations(ws *models.Workspace) error {
	automations := ws.Config.Automations
	if len(automations) == 0 && ws.Config.CronSchedule != "" {
		automations = []*models.Automation{
			{
				Name:      "default",
				Trigger:   models.TriggerConfig{Type: "cron", Value: ws.Config.CronSchedule},
				TaskFile:  "heartbeat.md",
				Strategy:  "persistent",
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

	jobFunc := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := d.executeAutomation(ctx, entry); err != nil {
			d.logger.Error("Automation execution failed",
				"workspace", entry.Workspace,
				"automation", entry.Name,
				"error", err)
		}
	}

	entryID, err := d.cron.AddFunc("@every 1m", jobFunc)
	if err != nil {
		return fmt.Errorf("failed to add cron function: %w", err)
	}

	d.mu.Lock()
	d.jobs[entry.ID] = entryID
	d.mu.Unlock()

	return nil
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
		State:         state,
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
