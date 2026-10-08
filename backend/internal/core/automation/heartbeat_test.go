package automation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

// recordingExecutor captures each request and answers with a canned report or error.
type recordingExecutor struct {
	mockExecutor
	mu       sync.Mutex
	requests []ExecuteRequest
	report   string
	err      error
	done     chan struct{}
}

func newRecordingExecutor(report string, err error) *recordingExecutor {
	return &recordingExecutor{report: report, err: err, done: make(chan struct{}, 8)}
}

func (e *recordingExecutor) Execute(_ context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	e.mu.Lock()
	e.requests = append(e.requests, req)
	e.mu.Unlock()
	defer func() { e.done <- struct{}{} }()
	if e.err != nil {
		return nil, e.err
	}
	return &ExecuteResponse{State: req.State, Report: e.report, Output: e.report}, nil
}

func (e *recordingExecutor) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.requests)
}

// heartbeatDispatcher registers an enabled heartbeat in workspace "ws" whose heartbeat.md holds file.
func heartbeatDispatcher(t *testing.T, exec TaskExecutor, file string) (*Dispatcher, *AutomationEntry) {
	t.Helper()
	d := newLaneDispatcher(t, exec)
	if err := d.persistence.WriteTaskFile("ws", models.HeartbeatFilename, file); err != nil {
		t.Fatal(err)
	}
	if err := d.Register("ws", models.HeartbeatConfig{Enabled: true}.Automation()); err != nil {
		t.Fatal(err)
	}
	entry, _ := d.registry.Get("ws", models.HeartbeatAutomationName)
	return d, entry
}

func statusOf(t *testing.T, d *Dispatcher) *models.HeartbeatStatus {
	t.Helper()
	status, err := d.persistence.ReadHeartbeatStatus("ws")
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestHeartbeat_NoChecksSkipsWithoutAModelCall(t *testing.T) {
	for name, file := range map[string]string{
		"comments only (the starter)": prompts.DefaultHeartbeat,
		"empty file":                  "",
		"whitespace":                  "  \n",
	} {
		t.Run(name, func(t *testing.T) {
			exec := newRecordingExecutor("HEARTBEAT_OK", nil)
			d, entry := heartbeatDispatcher(t, exec, file)
			res, err := d.admitRun(entry, false, "")
			if err != nil || res.Status != TriggerSkipped {
				t.Fatalf("admitRun = %+v, %v; want skipped", res, err)
			}
			if exec.count() != 0 {
				t.Errorf("the model was called %d times with nothing to check", exec.count())
			}
			if s := statusOf(t, d); s == nil || s.Result != models.HeartbeatSkippedNoChecks {
				t.Errorf("status = %+v, want skipped_no_checks", s)
			}
		})
	}
}

func TestHeartbeat_SendsTheChecksAndTheReplyRulesButNoComments(t *testing.T) {
	exec := newRecordingExecutor("HEARTBEAT_OK", nil)
	d, entry := heartbeatDispatcher(t, exec, "<!-- private note -->\nWatch: new Claude releases\n")
	if _, err := d.admitRun(entry, false, ""); err != nil {
		t.Fatal(err)
	}
	<-exec.done
	task := exec.requests[0].TaskContent
	if !strings.Contains(task, "Watch: new Claude releases") || !strings.Contains(task, prompts.HeartbeatReplyRules) {
		t.Errorf("task = %q, want the check and the reply rules", task)
	}
	if strings.Contains(task, "private note") {
		t.Errorf("a comment reached the model: %q", task)
	}
}

func TestHeartbeat_RecordsHowTheLastCheckEnded(t *testing.T) {
	cases := []struct {
		name   string
		report string
		err    error
		want   models.HeartbeatResult
	}{
		{"nothing to report", "HEARTBEAT_OK", nil, models.HeartbeatQuiet},
		{"an alert", "GPT-6 shipped: https://example.com/x", nil, models.HeartbeatAlert},
		{"a failed run", "", errors.New("model unavailable"), models.HeartbeatError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := newRecordingExecutor(tc.report, tc.err)
			d, entry := heartbeatDispatcher(t, exec, "Watch: releases")
			if _, err := d.admitRun(entry, false, ""); err != nil {
				t.Fatal(err)
			}
			<-exec.done
			waitFor(t, "the status to be written", func() bool { s := statusOf(t, d); return s != nil && s.Result == tc.want })
		})
	}
}

// A tick that fails before the executor starts (the checks file emptied between
// admission and preparation) must still show as an error, not a stale status.
func TestHeartbeat_PrepareFailureShowsAsError(t *testing.T) {
	d, entry := heartbeatDispatcher(t, newRecordingExecutor("HEARTBEAT_OK", nil), "Watch: releases")
	d.reportPrepareFailure(entry, errors.New("task file missing"), time.Now())
	if s := statusOf(t, d); s == nil || s.Result != models.HeartbeatError {
		t.Fatalf("status = %+v, want an error", s)
	}
}

// Only a local model contends for the GPU, so only then is a tick dropped instead of queued. An empty model
// follows the registry primary, so the lane is resolved at fire time.
func TestHeartbeat_SkipIfBusyOnlyOnTheLocalLane(t *testing.T) {
	d := newLaneDispatcher(t, &mockExecutor{})
	heartbeat := &AutomationEntry{Name: models.HeartbeatAutomationName}
	regular := &AutomationEntry{Name: "nightly", SkipIfBusy: true}
	plain := &AutomationEntry{Name: "plain"}

	cases := []struct {
		entry *AutomationEntry
		lane  runlane.LaneKey
		want  bool
	}{
		{heartbeat, runlane.LaneLocal, true},
		{heartbeat, runlane.LaneCloud, false},
		{regular, runlane.LaneCloud, true},
		{plain, runlane.LaneLocal, false},
	}
	for _, tc := range cases {
		d.laneKeyFor = func(string) runlane.LaneKey { return tc.lane }
		if got := d.skipIfBusy(tc.entry); got != tc.want {
			t.Errorf("skipIfBusy(%s on %s) = %v, want %v", tc.entry.Name, tc.lane, got, tc.want)
		}
	}
}

func TestHeartbeat_StateTellsTheUIWhatItNeedsToWarn(t *testing.T) {
	d := newLaneDispatcher(t, &mockExecutor{})
	if err := d.persistence.WriteTaskFile("ws", models.HeartbeatFilename, prompts.DefaultHeartbeat); err != nil {
		t.Fatal(err)
	}
	cfg := &models.WorkspaceConfig{Heartbeat: &models.HeartbeatConfig{Enabled: true, Model: "qwen"}}
	if err := d.persistence.WriteConfig("ws", cfg); err != nil {
		t.Fatal(err)
	}

	d.laneKeyFor = func(string) runlane.LaneKey { return runlane.LaneLocal }
	state, err := d.HeartbeatState("ws")
	if err != nil {
		t.Fatal(err)
	}
	if !state.WakesLocalModel || state.Lane != string(runlane.LaneLocal) || state.HasChecks || state.Status != nil || !state.Config.Enabled {
		t.Errorf("state = %+v, want local lane, no checks, no status yet", state)
	}

	if err := d.persistence.WriteTaskFile("ws", models.HeartbeatFilename, "Watch: releases"); err != nil {
		t.Fatal(err)
	}
	d.laneKeyFor = func(string) runlane.LaneKey { return runlane.LaneCloud }
	state, _ = d.HeartbeatState("ws")
	if state.WakesLocalModel || state.Lane != string(runlane.LaneCloud) || !state.HasChecks {
		t.Errorf("state = %+v, want cloud lane and checks present", state)
	}
}

// saveActiveHours stores the workspace heartbeat config the admission check reads at fire time.
func saveActiveHours(t *testing.T, d *Dispatcher, hours string) {
	t.Helper()
	cfg := &models.WorkspaceConfig{Heartbeat: &models.HeartbeatConfig{Enabled: true, ActiveHours: hours}}
	if err := d.persistence.WriteConfig("ws", cfg); err != nil {
		t.Fatal(err)
	}
}

// Active hours are enforced at admission: a scheduled tick outside the window is
// dropped before the lane (no model call, status recorded), the window edges are
// start-inclusive/end-exclusive, and a manual run ignores the window.
func TestHeartbeat_ActiveHoursGateScheduledTicks(t *testing.T) {
	plus5 := time.FixedZone("UTC+5", 5*60*60)
	at := func(h, m int, loc *time.Location) time.Time { return time.Date(2026, 10, 8, h, m, 0, 0, loc) }

	for _, tc := range []struct {
		name    string
		hours   string
		now     time.Time
		manual  bool
		wantRun bool
	}{
		{"no window always runs", "", at(3, 0, time.UTC), false, true},
		{"a minute before start is skipped", "08:00-22:00", at(7, 59, time.UTC), false, false},
		{"exactly at start runs", "08:00-22:00", at(8, 0, time.UTC), false, true},
		{"a minute before end runs", "08:00-22:00", at(21, 59, time.UTC), false, true},
		{"exactly at end is skipped", "08:00-22:00", at(22, 0, time.UTC), false, false},
		{"wrapping window late evening runs", "22:00-06:00", at(23, 0, time.UTC), false, true},
		{"wrapping window early morning runs", "22:00-06:00", at(5, 0, time.UTC), false, true},
		{"wrapping window midday is skipped", "22:00-06:00", at(12, 0, time.UTC), false, false},
		{"reads the wall clock of the clock's zone", "08:00-22:00", at(23, 0, plus5), false, false},
		{"a manual run outside the window still runs", "08:00-22:00", at(3, 0, time.UTC), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newRecordingExecutor("HEARTBEAT_OK", nil)
			d, entry := heartbeatDispatcher(t, exec, "Watch: new Claude releases\n")
			saveActiveHours(t, d, tc.hours)
			d.now = func() time.Time { return tc.now }

			res, err := d.admitRun(entry, tc.manual, "")
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantRun {
				if res.Status != TriggerSkipped || exec.count() != 0 {
					t.Fatalf("admitRun = %+v with %d model calls; want skipped without a model call", res, exec.count())
				}
				if s := statusOf(t, d); s == nil || s.Result != models.HeartbeatSkippedOutsideHours {
					t.Errorf("status = %+v, want skipped_outside_hours", s)
				}
				return
			}
			if res.Status == TriggerSkipped {
				t.Fatalf("admitRun skipped a tick that should run: %+v", res)
			}
			<-exec.done
		})
	}
}

// A scheduled tick admitted inside the window but queued behind other work must
// re-check the window when it leaves the queue: if the window closed meanwhile,
// it is dropped there instead of running (and maybe alerting) off-hours.
func TestHeartbeat_QueuedTickRechecksActiveHoursAtDequeue(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	d.laneKeyFor = func(string) runlane.LaneKey { return runlane.LaneCloud } // cloud: a busy lane queues the tick
	if err := d.persistence.WriteTaskFile("ws", models.HeartbeatFilename, "Watch: new Claude releases\n"); err != nil {
		t.Fatal(err)
	}
	if err := d.Register("ws", models.HeartbeatConfig{Enabled: true}.Automation()); err != nil {
		t.Fatal(err)
	}
	hb, _ := d.registry.Get("ws", models.HeartbeatAutomationName)
	saveActiveHours(t, d, "08:00-22:00")
	registerManualAutomation(t, d, "ws", "a")

	inside := time.Date(2026, 10, 8, 21, 59, 0, 0, time.UTC)
	var now atomic.Value
	now.Store(inside)
	d.now = func() time.Time { return now.Load().(time.Time) }

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "a") // the lane is now busy

	res, err := d.admitRun(hb, false, "")
	if err != nil || res.Status != TriggerQueued {
		t.Fatalf("tick inside the window behind a busy lane = %+v, %v; want queued", res, err)
	}

	now.Store(time.Date(2026, 10, 8, 23, 30, 0, 0, time.UTC)) // the window closes while it waits
	close(exec.proceed)

	select {
	case name := <-exec.started:
		t.Fatalf("%q ran after the window closed", name)
	case <-time.After(500 * time.Millisecond):
	}
	eventuallySettled(t, func() bool {
		s, _ := d.persistence.ReadHeartbeatStatus("ws")
		return s != nil && s.Result == models.HeartbeatSkippedOutsideHours
	}, "the dequeued tick was not recorded as skipped_outside_hours")
}
