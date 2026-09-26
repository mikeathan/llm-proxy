package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/models"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

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

	// scheduleAutomation atomically replaces any prior entry for this
	// automation, so a re-register (trigger type or schedule edit) cannot
	// orphan the old cron job and leave it firing alongside the new one.
	return d.scheduleAutomation(entry)
}

// removeJob stops and forgets the cron entry for an automation ID. It is safe
// to call for automations that were never scheduled (e.g. manual triggers).
func (d *Dispatcher) removeJob(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if entryID, exists := d.jobs[id]; exists {
		d.cron.Remove(entryID)
		delete(d.jobs, id)
	}
}

// replaceJob atomically removes the automation's existing cron entry and
// installs jobFn on schedule. Holding d.mu across the remove and the add stops
// two concurrent re-registers (e.g. an HTTP edit racing the config watcher)
// from each installing an entry and orphaning the other.
func (d *Dispatcher) replaceJob(id, schedule string, jobFn func()) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entryID, exists := d.jobs[id]; exists {
		d.cron.Remove(entryID)
		delete(d.jobs, id)
	}

	entryID, err := d.cron.AddFunc(schedule, jobFn)
	if err != nil {
		return fmt.Errorf("failed to add cron function: %w", err)
	}
	d.jobs[id] = entryID
	return nil
}

func (d *Dispatcher) Unregister(workspaceID, automationName string) error {
	entry, ok := d.registry.Get(workspaceID, automationName)
	if !ok {
		return fmt.Errorf("automation not found")
	}

	d.removeJob(entry.ID)

	d.registry.Unregister(workspaceID, automationName)
	return nil
}

// UnregisterWorkspace removes all scheduled automations for a given workspace.
func (d *Dispatcher) UnregisterWorkspace(workspaceID string) {
	d.registry.UnregisterWorkspace(workspaceID)

	d.mu.Lock()
	for id, entryID := range d.jobs {
		if strings.HasPrefix(id, workspaceID+"/") {
			d.cron.Remove(entryID)
			delete(d.jobs, id)
		}
	}
	d.mu.Unlock()
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
	if entry.Trigger.Type() == models.TriggerManual {
		d.removeJob(entry.ID)
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

		// Admit to the run lane: started now or queued FIFO behind running
		// work. The run timeout starts at dequeue — the lane owns the run ctx.
		// An already-queued fire is absorbed by admitRun, not an error.
		if _, err := d.admitRun(entry, false, ""); err != nil {
			d.logger.Error("Automation admission failed",
				"workspace", entry.Workspace,
				"automation", entry.Name,
				"error", err)
		}
	}

	return d.replaceJob(entry.ID, schedule, jobFunc)
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

// ============================================================================
// fsnotify Hot-Reload
// ============================================================================
func (d *Dispatcher) startWatcher(ctx context.Context) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	root := d.persistence.MetadataRoot()
	if err := watcher.Add(root); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch metadata directory: %w", err)
	}
	d.syncWorkspaceWatches(watcher, root)
	d.logger.Info("Started fsnotify watcher", "path", root)
	return watcher, nil
}

// syncWorkspaceWatches adds a watch for every workspace metadata directory so
// config.yaml writes are observed. fsnotify is non-recursive, so watching the
// metadata root alone misses writes inside per-workspace subdirectories.
func (d *Dispatcher) syncWorkspaceWatches(watcher *fsnotify.Watcher, root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		d.logger.Warn("Failed to list metadata directory for watching", "path", root, "error", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if err := watcher.Add(dir); err != nil {
			d.logger.Warn("Failed to watch workspace metadata directory", "path", dir, "error", err)
		}
	}
}

func (d *Dispatcher) watchConfigChanges(ctx context.Context, watcher *fsnotify.Watcher) error {
	root := d.persistence.MetadataRoot()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only react to config.yaml changes
			if filepath.Base(event.Name) == models.ConfigFilename {
				d.logger.Info("Config change detected, reconciling automations", "file", event.Name)
				d.handleConfigChange()
				continue
			}
			// A new workspace metadata directory appeared; watch it so its later
			// config.yaml writes are observed (fsnotify is not recursive).
			if event.Op&fsnotify.Create != 0 && filepath.Dir(event.Name) == root {
				d.syncWorkspaceWatches(watcher, root)
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
			if strings.HasPrefix(id, ws.ID+"/") {
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
		if wsID, _, found := strings.Cut(id, "/"); found && !activeIDs[wsID] {
			d.cron.Remove(d.jobs[id])
			delete(d.jobs, id)
			d.logger.Info("Removed deleted workspace from dispatcher", "workspace", wsID)
		}
	}
	d.mu.Unlock()
}
