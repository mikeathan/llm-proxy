package automation

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type mockExecutor struct{}

func (e *mockExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	return &ExecuteResponse{State: req.State}, nil
}

func (e *mockExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}

func (e *mockExecutor) ModelTimeout(modelName string) time.Duration { return 0 }

// newTestLane builds the lane every dispatcher test admits runs through,
// tethered to the test process root like production wiring does.
func newTestLane() *runlane.Scheduler {
	lane := runlane.New(runlane.Limits{Local: 1, Cloud: 1}, true)
	lane.Start(context.Background())
	return lane
}

func testLaneKeyFor(string) runlane.LaneKey { return runlane.LaneLocal }

func TestDispatcher_Start_CleanupStaleState(t *testing.T) {
	// 1. Setup temporary test environment
	tmpRoot, err := os.MkdirTemp("", "dispatcher-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpRoot)

	wsDir := filepath.Join(tmpRoot, "workspaces")
	os.MkdirAll(wsDir, 0755)

	resolver := storage.NewPathResolver(tmpRoot, wsDir, wsDir)
	manager := persistence.NewWorkspaceManager(resolver)
	logger := logging.NewNopLogger()

	// 2. Create a workspace with STALE "IsRunning" state
	wsID := "stale-ws"
	wsPath := resolver.WorkspaceDir(wsID)
	os.MkdirAll(filepath.Join(wsPath, ".internal"), 0755)

	staleState := &models.AgentState{}
	staleState.SetRunning("some-automation")
	if err := manager.WriteState(wsID, staleState); err != nil {
		t.Fatalf("failed to write stale state: %v", err)
	}

	// 3. Initialize Dispatcher
	d, err := NewDispatcher(DispatcherDeps{Persistence: manager, Executor: &mockExecutor{}, Logger: logger, Lane: newTestLane(), LaneKeyFor: testLaneKeyFor})
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}

	// 4. Start Dispatcher (this should trigger cleanup)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We don't need it to run for long, just long enough for Start to process workspaces
	go func() {
		if err := d.Start(ctx); err != nil {
			// Start returns error when context is cancelled, which is fine
		}
	}()

	// Give it a moment to process
	deadline := 10
	for i := 0; i < deadline; i++ {
		state, err := manager.ReadState(wsID)
		if err == nil && !state.IsRunning() {
			// Success! State was cleaned up.
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("Dispatcher.Start() failed to cleanup stale execution state for workspace %s", wsID)
}

// pgidMockExecutor returns a fixed PGID for ShellPGID queries.
type pgidMockExecutor struct {
	executeResp *ExecuteResponse
	pgid        int
	pgidErr     error
}

func (e *pgidMockExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	if e.executeResp != nil {
		return e.executeResp, nil
	}
	return &ExecuteResponse{State: req.State}, nil
}

func (e *pgidMockExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	return e.pgid, e.pgidErr
}

func (e *pgidMockExecutor) ModelTimeout(modelName string) time.Duration { return 0 }

// cancelDiagnostic cancels the StopAutomation diagnostic goroutine for a
// workspace. StopAutomation leaves that goroutine sleeping until the
// diagnostic delay (default 30s); cancelling it terminates the goroutine
// promptly so a test that calls StopAutomation does not leak it.
func cancelDiagnostic(t *testing.T, d *Dispatcher, workspaceID string) {
	t.Helper()
	d.runMu.Lock()
	r, ok := d.activeRuns[workspaceID]
	d.runMu.Unlock()
	if ok && r.diagCancel != nil {
		r.diagCancel()
	}
}

func TestStopAutomation_ForceKillUsesPGID(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
		executor:   &pgidMockExecutor{pgid: -12345},
		lane:       newTestLane(),
	}
	// StopAutomation spawns a diagnostic goroutine that sleeps for the default
	// 30s delay; cancel it so the test terminates the goroutine promptly instead
	// of leaving it to linger until the delay elapses.
	defer cancelDiagnostic(t, d, "test-ws")

	_, cancel := context.WithCancel(context.Background())
	d.activeRuns["test-ws"] = &activeRun{cancel: cancel, pgid: -12345}

	err := d.StopAutomation("test-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The goroutine will fire in 30s — the run is still in activeRuns
	// with a valid pgid. We can't assert syscall.Kill in a unit test
	// without a real process, but we can verify the run is still tracked.
	d.runMu.Lock()
	r, ok := d.activeRuns["test-ws"]
	d.runMu.Unlock()
	if !ok {
		t.Fatal("run removed prematurely")
	}
	if r.pgid != -12345 {
		t.Errorf("pgid changed: got %d, want -12345", r.pgid)
	}
}

func TestStopAutomation_NoShellGraceful(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
		executor:   &pgidMockExecutor{pgidErr: fmt.Errorf("no shell")},
		lane:       newTestLane(),
	}
	defer cancelDiagnostic(t, d, "test-ws")

	_, cancel := context.WithCancel(context.Background())
	d.activeRuns["test-ws"] = &activeRun{cancel: cancel}

	err := d.StopAutomation("test-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PGID should be 0 — graceful degradation
	d.runMu.Lock()
	r, ok := d.activeRuns["test-ws"]
	d.runMu.Unlock()
	if !ok {
		t.Fatal("run removed prematurely")
	}
	if r.pgid != 0 {
		t.Errorf("expected pgid=0 (no shell), got %d", r.pgid)
	}
}

func TestStopAutomation_NoActiveRun(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
		lane:       newTestLane(),
	}

	err := d.StopAutomation("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent run")
	}
}

func TestStopAutomation_ReplacedRunNoStaleKill(t *testing.T) {
	d := &Dispatcher{
		logger:          logging.NewNopLogger(),
		activeRuns:      make(map[string]*activeRun),
		diagnosticDelay: 0,

		lane: newTestLane(),
	}
	d.events = eventbus.NewBus()

	_, cancelA := context.WithCancel(context.Background())
	runA := &activeRun{cancel: cancelA, pgid: -11111}
	d.activeRuns["ws"] = runA

	_, cancelB := context.WithCancel(context.Background())
	runB := &activeRun{cancel: cancelB, pgid: -22222}

	err := d.StopAutomation("ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Replace runA with runB before diagnostic goroutine checks.
	d.runMu.Lock()
	d.activeRuns["ws"] = runB
	d.runMu.Unlock()

	// Give goroutine time to fire (delay=0).
	time.Sleep(50 * time.Millisecond)

	d.runMu.Lock()
	current := d.activeRuns["ws"]
	d.runMu.Unlock()
	if current != runB {
		t.Error("replacement run must not be deleted or replaced by stale diagnostic goroutine")
	}

	cancelA()
	cancelB()
}

func TestDefaultTaskExecutor_ShellPGID(t *testing.T) {
	e := &DefaultTaskExecutor{}
	pgid, err := e.ShellPGID(context.Background(), "ws")
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, ErrShellPGIDNotAvailable) {
		t.Errorf("expected ErrShellPGIDNotAvailable, got %v", err)
	}
	if pgid != 0 {
		t.Errorf("expected 0, got %d", pgid)
	}
}

func TestDispatcher_Stop_RespectsContext(t *testing.T) {
	d, err := NewDispatcher(DispatcherDeps{Executor: &mockExecutor{}, Logger: logging.NewStderrLogger(logging.LevelError), Lane: newTestLane(), LaneKeyFor: testLaneKeyFor})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	// A cancelled context must make Stop return promptly rather than waiting on
	// cron teardown. This guards the bounded-shutdown contract: shutdown must
	// never stall past the caller's deadline.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		d.Stop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success — Stop returned without blocking on the context deadline.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked past the cancelled context deadline")
	}

	// Stop must be idempotent and safe to call again.
	d.Stop(context.Background())
}

// TestTriggerToCron_RealSchedule verifies triggers register their actual
// schedule instead of a @every-1m poll (which retried pre-executor failures
// every minute and fired new automations immediately).
func TestTriggerToCron_RealSchedule(t *testing.T) {
	d := &Dispatcher{
		lane: newTestLane(),
	}
	cronTr, err := NewCronTrigger("0 0 * * *")
	if err != nil {
		t.Fatalf("cron trigger: %v", err)
	}
	intervalTr, err := NewIntervalTrigger("2h")
	if err != nil {
		t.Fatalf("interval trigger: %v", err)
	}
	if got := d.triggerToCron(cronTr); got != "0 0 * * *" {
		t.Errorf("cron triggerToCron = %q, want real expression", got)
	}
	want := "@every " + intervalTr.Value()
	if got := d.triggerToCron(intervalTr); got != want {
		t.Errorf("interval triggerToCron = %q, want %q", got, want)
	}
}

// TestRunContext_ModelTimeoutWins verifies the effective run bound is
// max(dispatcher cap, pinned model timeout), so a slow model configured with a
// longer timeout_minutes is not cut off by the dispatcher cap.
func TestRunContext_ModelTimeoutWins(t *testing.T) {
	slowTimeout := 30 * time.Minute
	d, err := NewDispatcher(DispatcherDeps{Executor: &slowModelExecutor{timeout: slowTimeout}, Logger: logging.NewNopLogger(), Lane: newTestLane(), LaneKeyFor: testLaneKeyFor})
	if err != nil {
		t.Fatalf("dispatcher: %v", err)
	}
	defer d.Stop(context.Background())

	entry := &AutomationEntry{Name: "a", Model: "slow-model"}
	ctx, cancel := d.runContext(context.Background(), entry)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected a deadline on the run context")
	}
	if got := time.Until(deadline); got < slowTimeout-time.Second || got > slowTimeout+time.Second {
		t.Errorf("expected deadline ~%v (model timeout), got %v", slowTimeout, got)
	}

	// No model override -> dispatcher cap applies.
	entryNoModel := &AutomationEntry{Name: "b"}
	ctx2, cancel2 := d.runContext(context.Background(), entryNoModel)
	defer cancel2()
	if d2, ok := ctx2.Deadline(); !ok || time.Until(d2) > defaultAutomationTimeout+time.Second {
		t.Errorf("expected dispatcher cap without model override, got deadline %v", time.Until(d2))
	}
}

type slowModelExecutor struct {
	mockExecutor
	timeout time.Duration
}

func (e *slowModelExecutor) ModelTimeout(modelName string) time.Duration { return e.timeout }

// blockingExecutor models the real executor: the run occupies its lane slot
// until the test releases it, and a cancelled run appends the failed history
// entry the real executor writes before returning an error.
type blockingExecutor struct {
	started chan string
	proceed chan struct{}
	calls   atomic.Int64
}

func (e *blockingExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	e.calls.Add(1)
	e.started <- req.AutomationName
	select {
	case <-e.proceed:
		return &ExecuteResponse{State: req.State}, nil
	case <-ctx.Done():
		if req.State != nil {
			req.State.History = append(req.State.History, models.AutomationRun{
				WorkspaceID:    req.WorkspaceID,
				AutomationName: req.AutomationName,
				Error:          "context canceled",
			})
		}
		return nil, ctx.Err()
	}
}

func (e *blockingExecutor) ShellPGID(context.Context, string) (int, error) { return 0, nil }
func (e *blockingExecutor) ModelTimeout(string) time.Duration              { return 0 }

// newLaneDispatcher builds a dispatcher over a temp workspace root with a
// local-first lane, matching production wiring.
func newLaneDispatcher(t *testing.T, exec TaskExecutor) *Dispatcher {
	t.Helper()
	tmpRoot, err := os.MkdirTemp("", "dispatcher-lane-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpRoot) })
	wsDir := filepath.Join(tmpRoot, "workspaces")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := storage.NewPathResolver(tmpRoot, wsDir, wsDir)
	d, err := NewDispatcher(DispatcherDeps{
		Persistence: persistence.NewWorkspaceManager(resolver),
		Executor:    exec,
		Logger:      logging.NewNopLogger(),
		Lane:        runlane.New(runlane.Limits{Local: 1, Cloud: 1}, true),
		LaneKeyFor:  testLaneKeyFor,
	})
	if err != nil {
		t.Fatal(err)
	}
	d.lane.Start(context.Background())
	return d
}

func registerManualAutomation(t *testing.T, d *Dispatcher, ws, name string) {
	t.Helper()
	if err := d.Register(ws, &models.Automation{
		Name:     name,
		Trigger:  models.TriggerConfig{Type: "manual"},
		TaskFile: "task.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.persistence.WriteTaskFile(ws, "task.md", "run the task"); err != nil {
		t.Fatal(err)
	}
}

func waitStarted(t *testing.T, started <-chan string, want string) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("started %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %q to start", want)
	}
}

func eventuallySettled(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestDispatcher_Lane_OverlappingFiresQueueInsteadOfDropping(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")

	res, err := d.Trigger("ws", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TriggerQueued || res.Position != 1 {
		t.Fatalf("res = %+v, want queued at position 1", res)
	}

	// A third fire while it is already queued is absorbed — reported as queued
	// at the same position, never an error.
	res, err = d.Trigger("ws", "a", "")
	if err != nil {
		t.Fatalf("trigger while queued = %v, want nil", err)
	}
	if res.Status != TriggerQueued || res.Position != 1 {
		t.Fatalf("res = %+v, want queued at position 1", res)
	}

	close(exec.proceed)
	waitStarted(t, exec.started, "a") // the queued fire becomes the pending rerun
	if got := atomic.LoadInt64(&d.metrics.QueuedExecutions); got != 1 {
		t.Fatalf("queued executions = %d, want 1", got)
	}
}

func TestDispatcher_Lane_CancelQueuedLeavesRunningRunAlone(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")

	if err := d.CancelQueued("ws", "a"); !errors.Is(err, ErrNoQueuedRun) {
		t.Fatalf("CancelQueued on a running run = %v, want ErrNoQueuedRun", err)
	}
	if ls := d.LaneSnapshot().Lanes[0]; ls.Running != 1 {
		t.Fatalf("running = %d, want the run left untouched", ls.Running)
	}
	if got := atomic.LoadInt64(&d.metrics.FailedExecutions); got != 0 {
		t.Fatalf("failed executions = %d, want 0", got)
	}

	close(exec.proceed)
	eventuallySettled(t, func() bool { return d.LaneSnapshot().Lanes[0].Running == 0 },
		"running run never released its slot")
}

func TestDispatcher_Lane_PreemptedRunIsSkippedNotFailed(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")

	events, _ := d.Events().Subscribe("ws", assistant.ChannelAutomation)

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")

	// The interactive claim preempts the running automation; the claim is only
	// granted after the run unwound, so the run settled when this returns.
	chatCtx, release, err := d.lane.ClaimInteractive(context.Background(), runlane.LaneLocal, "ws", "m1")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if runlane.Preempted(chatCtx) {
		t.Fatal("interactive run ctx must not carry a preemption marker")
	}

	state, err := d.persistence.ReadState("ws")
	if err != nil {
		t.Fatal(err)
	}
	if state.IsRunning() {
		t.Fatal("preempted run left the workspace marked running")
	}
	if len(state.History) != 0 {
		t.Fatalf("history = %+v, want the failed tail entry dropped", state.History)
	}
	if got := atomic.LoadInt64(&d.metrics.PreemptedExecutions); got != 1 {
		t.Fatalf("preempted executions = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&d.metrics.FailedExecutions); got != 0 {
		t.Fatalf("failed executions = %d, want 0", got)
	}
	for {
		select {
		case ev := <-events:
			if ev.Type == assistant.EventError {
				t.Fatalf("preempted run published %q", ev.Type)
			}
			continue
		default:
		}
		break
	}
	close(exec.proceed)
}

func TestDispatcher_Lane_SameWorkspaceRunsQueueOnFlock(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")
	registerManualAutomation(t, d, "ws", "b")

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")
	if _, err := d.Trigger("ws", "b", ""); err != nil {
		t.Fatal(err)
	}

	close(exec.proceed) // a finishes; queued b starts and waits for the flock
	waitStarted(t, exec.started, "b")
	if got := atomic.LoadInt64(&d.metrics.SkippedExecutions); got != 0 {
		t.Fatalf("skipped executions = %d, want 0 (the flock must queue, not drop)", got)
	}
}

func TestDispatcher_Lane_StopAutomationClearsQueuedEntries(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")
	registerManualAutomation(t, d, "ws", "b")

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")
	if _, err := d.Trigger("ws", "b", ""); err != nil {
		t.Fatal(err)
	}

	if err := d.StopAutomation("ws"); err != nil {
		t.Fatal(err)
	}
	err := d.CancelQueued("ws", "b")
	if !errors.Is(err, ErrNoQueuedRun) {
		t.Fatalf("err = %v, want ErrNoQueuedRun (Stop clears pending work)", err)
	}
	close(exec.proceed)
	eventuallySettled(t, func() bool { return d.LaneSnapshot().Lanes[0].Running == 0 },
		"stopped run never released its slot")
}

func TestDispatcher_Lane_DropsQueuedEntryForDeletedAutomation(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	registerManualAutomation(t, d, "ws", "a")
	registerManualAutomation(t, d, "ws", "b")

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a")
	if _, err := d.Trigger("ws", "b", ""); err != nil {
		t.Fatal(err)
	}
	d.Unregister("ws", "b")

	close(exec.proceed) // a finishes; the queued b re-resolves to nothing and drops
	eventuallySettled(t, func() bool {
		ls := d.LaneSnapshot().Lanes[0]
		return ls.Running == 0 && len(ls.Queued) == 0
	}, "lane did not drain after the deleted automation was dropped")
	if got := exec.calls.Load(); got != 1 {
		t.Fatalf("executions observed = %d, want 1 (the stale entry must not run)", got)
	}
}

func TestDispatcherMetrics_Snapshot(t *testing.T) {
	m := &DispatcherMetrics{}
	m.RecordExecution(true, false, 1500*time.Millisecond)
	m.RecordExecution(false, true, 500*time.Millisecond)
	m.RecordQueued()
	m.RecordPreempted()

	snap := m.Snapshot()
	if snap.TotalExecutions != 2 || snap.SuccessfulExecutions != 1 || snap.SkippedExecutions != 1 {
		t.Fatalf("snapshot counters = %+v, want 2 total / 1 successful / 1 skipped", snap)
	}
	if snap.QueuedExecutions != 1 || snap.PreemptedExecutions != 1 {
		t.Fatalf("snapshot admission counters = %+v, want 1 queued / 1 preempted", snap)
	}
	if snap.TotalLatency != 2*time.Second {
		t.Fatalf("snapshot latency = %v, want 2s", snap.TotalLatency)
	}
}
