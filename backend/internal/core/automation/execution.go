package automation

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"syscall"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/failures"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/safe"
	"llm-proxy/models"
)

// MsgRunPausedForInteractive is published when a lane preemption pauses a
// running automation for an interactive chat; the run restarts from its task
// file when the lane frees.
const MsgRunPausedForInteractive = "Run paused for an interactive request — it will restart when the lane is free."

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
	defer d.recoverRunPanic(entry, start, &retErr)

	execCtx, cancel := d.runContext(ctx, entry)
	defer cancel()

	f, err := d.acquireWorkspaceLock(execCtx, entry.Workspace)
	if err != nil {
		// Waiting for the workspace flock consumed the run ctx: the run never
		// started, so it counts as skipped — never failed (no history entry).
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

	stratCtx, state, taskContent, err := d.prepareRun(execCtx, entry)
	if err != nil {
		d.metrics.RecordExecution(false, false, time.Since(start))
		return err
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
		NetworkGrant:   entry.NetworkGrant,
	}

	resp, err := d.executor.Execute(stratCtx, req)
	elapsed := time.Since(start)

	if err != nil {
		return d.failRun(entry, state, execCtx, err, elapsed)
	}

	d.succeedRun(entry, state, resp)
	d.metrics.RecordExecution(true, false, elapsed)
	return nil
}

// recoverRunPanic contains a panic in a single run so a bad run can never crash
// the service; it clears the run's "running" state and records the failure so
// the workspace is not left marked running after a panic. Registered as a
// deferred call so recover() observes the panicking run.
func (d *Dispatcher) recoverRunPanic(entry *AutomationEntry, start time.Time, retErr *error) {
	rec := recover()
	if rec == nil {
		return
	}
	d.logger.Error("panic in automation run",
		"workspace", entry.Workspace, "automation", entry.Name,
		"error", fmt.Sprintf("%v", rec), "stack", string(debug.Stack()))
	if state, sErr := d.persistence.ReadState(entry.Workspace); sErr == nil {
		state.SetRunning("")
		_ = d.persistence.WriteState(entry.Workspace, state)
	}
	d.metrics.RecordExecution(false, false, time.Since(start))
	*retErr = fmt.Errorf("automation panicked: %v", rec)
}

// prepareRun reads the persisted state and task file, marks the workspace
// running, publishes the boot event and prepares the run strategy. The caller
// records the failure and clears the run when it returns an error.
func (d *Dispatcher) prepareRun(execCtx context.Context, entry *AutomationEntry) (context.Context, *models.AgentState, string, error) {
	state, err := d.persistence.ReadState(entry.Workspace)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read state: %w", err)
	}

	taskContent, err := d.persistence.ReadTaskFile(entry.Workspace, entry.TaskFile)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read task file %s: %w", entry.TaskFile, err)
	}
	if taskContent == "" {
		return nil, nil, "", fmt.Errorf("task file %s is empty", entry.TaskFile)
	}

	state.SetRunning(entry.Name)
	if err := d.persistence.WriteState(entry.Workspace, state); err != nil {
		return nil, nil, "", fmt.Errorf("failed to write state: %w", err)
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
		return nil, nil, "", fmt.Errorf("strategy preparation failed: %w", err)
	}
	return stratCtx, state, taskContent, nil
}

// failRun handles a failed executor return. A lane preemption is closed out as
// skipped (no failure); any other error clears the running state, publishes a
// classified error to the UI, records it in the ledger and is returned.
func (d *Dispatcher) failRun(entry *AutomationEntry, state *models.AgentState, execCtx context.Context, runErr error, elapsed time.Duration) error {
	if runlane.Preempted(execCtx) {
		d.recordPreempted(entry, state, elapsed)
		return nil
	}
	state.SetRunning("")
	d.persistence.WriteState(entry.Workspace, state)

	// Un-hang the UI by publishing the error over the EventBus. The content is
	// the classified summary + hint — the raw error would be an opaque upstream
	// JSON dump in the run console (full error is logged).
	fi := failures.ClassifyRunFailure(runErr)
	errContent := fi.Error
	if fi.Hint != "" {
		errContent = fi.Error + "\n\n" + fi.Hint
	}
	d.events.Publish(entry.Workspace, assistant.AgentEvent{
		Type:    assistant.EventError,
		Channel: assistant.ChannelAutomation,
		Payload: proxy.Message{
			Role:    "system",
			Content: errContent,
		},
	})

	// Ensure failed runs also propagate to the global ledger
	if len(state.History) > 0 {
		d.RecordActivity(state.History[len(state.History)-1])
	}

	d.metrics.RecordExecution(false, false, elapsed)
	return runErr
}

// succeedRun persists a successful run's outcome (pulse + cleared running
// state) and propagates it to the global ledger. A nil response leaves state
// untouched; the caller still records the execution as successful.
func (d *Dispatcher) succeedRun(entry *AutomationEntry, state *models.AgentState, resp *ExecuteResponse) {
	if resp == nil || resp.State == nil {
		return
	}
	if resp.Output != "" {
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
}

func (d *Dispatcher) StopAutomation(workspaceID string) error {
	// "Stop" clears pending work too: a queued entry would otherwise start
	// right after the running one is cancelled.
	d.cancelQueuedForWorkspace(workspaceID)

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

// recordPreempted closes out a run cancelled by a run-lane preemption: the
// executor already appended a failed history entry to the shared state (and
// RecordActivity would propagate it), so the tail entry is dropped, the run
// counts as skipped and the UI is informed instead of alarmed. The run
// restarts from its task file when the lane frees.
func (d *Dispatcher) recordPreempted(entry *AutomationEntry, state *models.AgentState, elapsed time.Duration) {
	if n := len(state.History); n > 0 && state.History[n-1].Error != "" {
		state.History = state.History[:n-1]
	}
	state.SetRunning("")
	// Best-effort, mirroring the error path: a failed write leaves the stale
	// running flag that Start clears on boot.
	_ = d.persistence.WriteState(entry.Workspace, state)
	d.metrics.RecordPreempted()
	d.metrics.RecordExecution(false, true, elapsed)
	d.events.Publish(entry.Workspace, assistant.AgentEvent{
		Type:    assistant.EventMessage,
		Channel: assistant.ChannelAutomation,
		Payload: proxy.Message{Role: "system", Content: MsgRunPausedForInteractive},
	})
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
