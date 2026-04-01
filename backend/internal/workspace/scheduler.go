package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"llm-proxy/models"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

// Scheduler manages scheduled execution of workspace heartbeats.
type Scheduler struct {
	manager  *Manager
	executor AgentExecutor
	cron     *cron.Cron
	logger   *slog.Logger

	mu      sync.RWMutex
	jobs    map[string]cron.EntryID
	watcher *fsnotify.Watcher
}

// NewScheduler initializes a new scheduler.
func NewScheduler(manager *Manager, executor AgentExecutor, logger *slog.Logger) (*Scheduler, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	return &Scheduler{
		manager:  manager,
		executor: executor,
		// cron parser supporting seconds, minutes, hours, dom, month, dow, and descriptors
		cron:    cron.New(cron.WithParser(cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))),
		logger:  logger,
		jobs:    make(map[string]cron.EntryID),
		watcher: watcher,
	}, nil
}

// Start initiates the cron scheduler and fsnotify file watcher. Blocks until ctx is canceled.
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting workspace scheduler")

	// 1. Initial Load of Workspaces
	workspaces, err := s.manager.ListWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to list workspaces on startup: %w", err)
	}

	for _, ws := range workspaces {
		if err := s.scheduleWorkspace(ws); err != nil {
			s.logger.Error("Failed to schedule workspace on startup", "workspace", ws.ID, "error", err)
		}
	}

	s.cron.Start()

	// 2. Watch Directory for Hot Reload
	if err := s.watcher.Add(s.manager.baseDir); err != nil {
		s.logger.Warn("Failed to watch workspaces base directory", "error", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)

	// File watcher loop
	eg.Go(func() error {
		for {
			select {
			case <-egCtx.Done():
				return nil
			case event, ok := <-s.watcher.Events:
				if !ok {
					return nil
				}
				// We don't want to react to every temporary lock file or state update
				// In a full implementation, we'd filter for "config.yaml" changes only.
				// For simplicity, handle it broadly.
				s.logger.Debug("FSEvent received", "event", event.String())
				s.handleFSEvent()
			case err, ok := <-s.watcher.Errors:
				if !ok {
					return nil
				}
				s.logger.Error("Watcher error", "error", err)
			}
		}
	})

	<-egCtx.Done()

	// 3. Graceful Shutdown
	s.logger.Info("Stopping scheduler gracefully...")
	s.watcher.Close()

	cronCtx := s.cron.Stop()
	select {
	case <-cronCtx.Done():
		s.logger.Info("All cron jobs finished successfully")
	case <-time.After(30 * time.Second):
		s.logger.Warn("Cron jobs did not finish within timeout")
	}

	return eg.Wait()
}

func (s *Scheduler) handleFSEvent() {
	// Re-list and reconcile
	workspaces, err := s.manager.ListWorkspaces()
	if err != nil {
		s.logger.Error("Failed to list workspaces during fs event", "error", err)
		return
	}

	activeIDs := make(map[string]bool)

	for _, ws := range workspaces {
		activeIDs[ws.ID] = true

		s.mu.Lock()
		existingEntry, exists := s.jobs[ws.ID]
		s.mu.Unlock()

		if exists {
			s.cron.Remove(existingEntry)
		}

		if err := s.scheduleWorkspace(ws); err != nil {
			s.logger.Error("Failed to reschedule workspace", "workspace", ws.ID, "error", err)
		}
	}

	// Remove deleted workspaces
	s.mu.Lock()
	for id, entryID := range s.jobs {
		if !activeIDs[id] {
			s.cron.Remove(entryID)
			delete(s.jobs, id)
			s.logger.Info("Removed deleted workspace from scheduler", "workspace", id)
		}
	}
	s.mu.Unlock()
}

// scheduleWorkspace assigns a cron entry to the workspace if it has a schedule.
func (s *Scheduler) scheduleWorkspace(ws *models.Workspace) error {
	if ws.Config.CronSchedule == "" {
		s.mu.Lock()
		delete(s.jobs, ws.ID)
		s.mu.Unlock()
		return nil
	}

	jobFunc := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := s.ExecuteHeartbeat(ctx, ws.ID); err != nil {
			s.logger.Error("Scheduled heartbeat failed", "workspace", ws.ID, "error", err)
		}
	}

	entryID, err := s.cron.AddFunc(ws.Config.CronSchedule, jobFunc)
	if err != nil {
		return fmt.Errorf("invalid cron schedule '%s': %w", ws.Config.CronSchedule, err)
	}

	s.mu.Lock()
	s.jobs[ws.ID] = entryID
	entry := s.cron.Entry(entryID)
	s.mu.Unlock()

	s.logger.Info("Scheduled workspace heartbeat", "workspace", ws.ID, "next_run", entry.Next)

	// Optimistically update NextRunPredicted
	f, err := s.manager.TryAcquireLock(ws.ID)
	if err == nil {
		defer s.manager.ReleaseLock(f)
		state, _ := s.manager.ReadState(ws.ID)
		if state == nil {
			state = &models.AgentState{}
		}
		state.NextRunPredicted = entry.Next
		_ = s.manager.WriteState(ws.ID, state)
	}

	return nil
}

// ExecuteHeartbeat performs a single execution safely. Can be triggered manually or via cron.
func (s *Scheduler) ExecuteHeartbeat(ctx context.Context, workspaceID string) error {
	// 1. Prevent concurrent runs using non-blocking lock
	f, err := s.manager.TryAcquireLock(workspaceID)
	if err != nil {
		return fmt.Errorf("heartbeat skipped (already running or locked): %w", err)
	}
	defer s.manager.ReleaseLock(f)

	// 2. Load State & Context
	state, err := s.manager.ReadState(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	prompt, err := s.manager.ReadHeartbeat(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to read heartbeat prompt: %w", err)
	}
	if prompt == "" {
		return fmt.Errorf("heartbeat prompt is empty")
	}

	// 3. Mark Running
	state.IsRunning = true
	_ = s.manager.WriteState(workspaceID, state)

	// 4. Delegate to Executor
	output, execErr := s.executor.Execute(ctx, prompt, state)

	// 5. Commit Final State
	state.IsRunning = false
	if execErr != nil {
		state.LastError = execErr.Error()
	} else {
		state.LastOutput = output
		state.LastError = "" // clear on success
	}

	// Update NextRunPredicted
	s.mu.RLock()
	if entryID, ok := s.jobs[workspaceID]; ok {
		entry := s.cron.Entry(entryID)
		state.NextRunPredicted = entry.Next
	}
	s.mu.RUnlock()

	return s.manager.WriteState(workspaceID, state)
}
