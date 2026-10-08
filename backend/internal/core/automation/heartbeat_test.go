package automation

import (
	"context"
	"errors"
	"strings"
	"sync"
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
