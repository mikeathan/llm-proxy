package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/models"
)

type scriptedExecutor struct {
	mu     sync.Mutex
	report string
	err    error
	tasks  []string
	reqs   []ExecuteRequest
}

func (e *scriptedExecutor) Execute(_ context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, req.TaskContent)
	e.reqs = append(e.reqs, req)
	if e.err != nil {
		return nil, e.err
	}
	runID := fmt.Sprintf("run_%d", len(e.reqs))
	if req.State != nil {
		req.State.AppendRun(models.AutomationRun{ID: runID, WorkspaceID: req.WorkspaceID, AutomationName: req.AutomationName, Output: e.report})
	}
	return &ExecuteResponse{State: req.State, Output: "header\n" + e.report, Report: e.report, RunID: runID}, nil
}

func (e *scriptedExecutor) ShellPGID(context.Context, string) (int, error) { return 0, nil }
func (e *scriptedExecutor) ModelTimeout(string) time.Duration              { return 0 }

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
	err  error
}

func (n *fakeNotifier) Notify(_ context.Context, connector, message string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if connector != "tg" {
		return errors.New("unknown connector " + connector)
	}
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, message)
	return nil
}

func newNotifyDispatcher(t *testing.T, exec TaskExecutor, n Notifier, cfg *models.NotifyConfig) (*Dispatcher, *AutomationEntry) {
	t.Helper()
	d := newLaneDispatcher(t, exec)
	d.notifier = n
	auto := &models.Automation{
		Name:     "nightly",
		Trigger:  models.TriggerConfig{Type: "manual"},
		TaskFile: "task.md",
		Notify:   cfg,
	}
	if err := d.Register("ws", auto); err != nil {
		t.Fatal(err)
	}
	if err := d.persistence.WriteTaskFile("ws", "task.md", "find releases"); err != nil {
		t.Fatal(err)
	}
	entry, _ := d.registry.Get("ws", "nightly")
	return d, entry
}

func TestDispatcher_NotifyDelivery(t *testing.T) {
	report := "| Item | Source |\n|---|---|\n| GPT-6 | https://openai.com/blog/gpt-6 |\n"
	cfg := &models.NotifyConfig{Connector: "tg", Dedup: true}

	t.Run("delivers once, then stays silent and hints the next run", func(t *testing.T) {
		exec := &scriptedExecutor{report: report}
		n := &fakeNotifier{}
		d, entry := newNotifyDispatcher(t, exec, n, cfg)

		for i := 0; i < 2; i++ {
			if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
				t.Fatal(err)
			}
		}
		if len(n.sent) != 1 || !strings.Contains(n.sent[0], "• GPT-6") {
			t.Fatalf("sent = %q, want exactly one digest with GPT-6", n.sent)
		}
		if strings.Contains(exec.tasks[0], "Already reported") {
			t.Error("first run must not carry an already-reported block")
		}
		if !strings.Contains(exec.tasks[1], "Already reported") || !strings.Contains(exec.tasks[1], "- GPT-6") {
			t.Errorf("second run task lacks the seen hint:\n%s", exec.tasks[1])
		}
	})

	t.Run("failed delivery is not recorded as seen", func(t *testing.T) {
		exec := &scriptedExecutor{report: report}
		n := &fakeNotifier{err: errors.New("telegram down")}
		d, entry := newNotifyDispatcher(t, exec, n, cfg)

		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatalf("delivery failure must not fail the run: %v", err)
		}
		if ledger, _ := d.persistence.ReadSeen("ws", "nightly"); len(ledger) != 0 {
			t.Errorf("undelivered items were recorded as seen: %v", ledger)
		}
	})

	t.Run("no notify block means no delivery", func(t *testing.T) {
		n := &fakeNotifier{}
		d, entry := newNotifyDispatcher(t, &scriptedExecutor{report: report}, n, nil)
		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatal(err)
		}
		if len(n.sent) != 0 {
			t.Errorf("unexpected delivery: %q", n.sent)
		}
	})
}

func TestDispatcher_NotifyFailure(t *testing.T) {
	cfg := &models.NotifyConfig{Connector: "tg"}

	t.Run("failed run is reported", func(t *testing.T) {
		n := &fakeNotifier{}
		d, entry := newNotifyDispatcher(t, &scriptedExecutor{err: errors.New("model exploded")}, n, cfg)
		if err := d.executeAutomation(context.Background(), entry, ""); err == nil {
			t.Fatal("expected the run error to propagate")
		}
		if len(n.sent) != 1 || !strings.Contains(n.sent[0], "nightly") || !strings.Contains(n.sent[0], "failed") {
			t.Errorf("sent = %q, want one failure notice naming the automation", n.sent)
		}
	})

	t.Run("user stop is not reported", func(t *testing.T) {
		n := &fakeNotifier{}
		d, entry := newNotifyDispatcher(t, &scriptedExecutor{err: context.Canceled}, n, cfg)
		_ = d.executeAutomation(context.Background(), entry, "")
		if len(n.sent) != 0 {
			t.Errorf("a cancelled run must not notify: %q", n.sent)
		}
	})
}

func TestDispatcher_NotifyHeartbeatOK(t *testing.T) {
	cases := []struct {
		name, report string
		wantSent     bool
	}{
		{"bare marker is quiet", "HEARTBEAT_OK", false},
		{"marker with a trailing note is quiet", "HEARTBEAT_OK\n\nNothing new today.", false},
		{"bold marker is quiet", "**HEARTBEAT_OK**", false},
		{"code-span marker is quiet", "`HEARTBEAT_OK`", false},
		{"an alert that merely mentions the marker is delivered", "GPT-6 shipped. Not HEARTBEAT_OK: https://openai.com/blog/gpt-6", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := &fakeNotifier{}
			d, entry := newNotifyDispatcher(t, &scriptedExecutor{report: tc.report}, n, &models.NotifyConfig{Connector: "tg"})
			if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
				t.Fatal(err)
			}
			if got := len(n.sent) == 1; got != tc.wantSent {
				t.Errorf("delivered = %v, want %v (sent %q)", got, tc.wantSent, n.sent)
			}
		})
	}
}

func TestDispatcher_SkipIfBusy(t *testing.T) {
	exec := &blockingExecutor{started: make(chan string, 8), proceed: make(chan struct{})}
	d := newLaneDispatcher(t, exec)
	if err := d.Register("ws", &models.Automation{
		Name: "beat", Trigger: models.TriggerConfig{Type: "manual"}, TaskFile: "task.md", SkipIfBusy: true,
	}); err != nil {
		t.Fatal(err)
	}
	registerManualAutomation(t, d, "ws", "other")
	entry, _ := d.registry.Get("ws", "beat")

	if _, err := d.Trigger("ws", "other", ""); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, exec.started, "other")

	res, err := d.admitRun(entry, false, "")
	if err != nil || res.Status != TriggerSkipped {
		t.Fatalf("scheduled fire on a busy lane = %+v, %v; want skipped", res, err)
	}
	for _, lane := range d.LaneSnapshot().Lanes {
		if len(lane.Queued) != 0 {
			t.Fatalf("a skipped tick must not be queued (lane %s): %+v", lane.Lane, lane.Queued)
		}
	}

	res, err = d.admitRun(entry, true, "")
	if err != nil || res.Status != TriggerQueued {
		t.Fatalf("a manual trigger must still queue, got %+v, %v", res, err)
	}
	close(exec.proceed)
}

func TestDispatcher_FailureNoticeIsRateLimited(t *testing.T) {
	exec := &scriptedExecutor{err: errors.New("model unavailable")}
	n := &fakeNotifier{}
	d, entry := newNotifyDispatcher(t, exec, n, &models.NotifyConfig{Connector: "tg"})
	run := func() { _ = d.executeAutomation(context.Background(), entry, "") }

	run()
	run()
	if len(n.sent) != 1 {
		t.Fatalf("repeated failures must send one notice, sent %q", n.sent)
	}

	// A successful run ends the outage: the next failure is news again.
	exec.mu.Lock()
	exec.err, exec.report = nil, "HEARTBEAT_OK"
	exec.mu.Unlock()
	run()
	exec.mu.Lock()
	exec.err = errors.New("model unavailable again")
	exec.mu.Unlock()
	run()
	if len(n.sent) != 2 {
		t.Fatalf("a failure after recovery must notify again, sent %q", n.sent)
	}
}

func TestFailureNoticeLimiter_CooldownExpires(t *testing.T) {
	var l failureNoticeLimiter
	now := time.Now()
	if !l.reserve("k", now) {
		t.Fatal("first notice must be allowed")
	}
	if l.reserve("k", now.Add(failureNoticeCooldown-time.Second)) {
		t.Error("notice inside the cooldown must be suppressed")
	}
	if !l.reserve("k", now.Add(failureNoticeCooldown)) {
		t.Error("notice after the cooldown must be allowed")
	}
	l.reset("k")
	if !l.reserve("k", now) {
		t.Error("reset must re-arm the notice")
	}
}

// Two failures of one automation at the same moment must send one notice, and a
// send that fails must not use up the cooldown.
func TestFailureNoticeLimiter_ReserveIsExclusiveAndReleasable(t *testing.T) {
	var l failureNoticeLimiter
	now := time.Now()
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.reserve("k", now) {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d concurrent failures each reserved a notice, want exactly 1", wins)
	}
	l.release("k")
	if !l.reserve("k", now) {
		t.Error("a failed send must give the reservation back")
	}
}

// lastRun reads the automation's latest run back from disk, as the UI does.
func lastRun(t *testing.T, d *Dispatcher) *models.AutomationRun {
	t.Helper()
	state, err := d.persistence.ReadState("ws")
	if err != nil {
		t.Fatal(err)
	}
	return state.LastRuns["nightly"]
}

// A report that could not be delivered must not look delivered: the run keeps its success (the work exists) but
// carries a warning naming the connector, on the run that produced the report. A delivered report, or nothing to
// deliver, adds no warning.
func TestDispatcher_DeliveryFailureIsRecordedOnTheRun(t *testing.T) {
	report := "| Item | Source |\n|---|---|\n| GPT-6 | https://openai.com/blog/gpt-6 |\n"
	cfg := &models.NotifyConfig{Connector: "tg"}

	t.Run("failed send", func(t *testing.T) {
		d, entry := newNotifyDispatcher(t, &scriptedExecutor{report: report}, &fakeNotifier{err: errors.New("telegram API error: status 401")}, cfg)
		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatalf("a delivery failure must not fail the run: %v", err)
		}
		run := lastRun(t, d)
		if run == nil || run.Error != "" || len(run.Warnings) != 1 ||
			!strings.Contains(run.Warnings[0], "tg") || !strings.Contains(run.Warnings[0], "status 401") {
			t.Fatalf("latest run = %+v, want one delivery warning naming the connector and the cause", run)
		}
		state, _ := d.persistence.ReadState("ws")
		if h := state.History[len(state.History)-1]; len(h.Warnings) != 1 {
			t.Errorf("history entry lacks the warning: %+v", h)
		}
	})

	for name, n := range map[string]*fakeNotifier{"delivered": {}, "nothing to deliver": {}} {
		t.Run(name, func(t *testing.T) {
			r := report
			if name == "nothing to deliver" {
				r = ""
			}
			d, entry := newNotifyDispatcher(t, &scriptedExecutor{report: r}, n, cfg)
			if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
				t.Fatal(err)
			}
			if run := lastRun(t, d); run != nil && len(run.Warnings) != 0 {
				t.Errorf("unexpected warnings: %v", run.Warnings)
			}
		})
	}
}

// A rejected bot token reads as a short reason in the run's delivery warning, not the platform's raw reply.
func TestDispatcher_DeliveryWarningUsesTheShortReason(t *testing.T) {
	rejected := &models.ToolUnavailableError{Reason: "telegram rejected the bot token (HTTP 401)", Cause: errors.New(`{"ok":false,"error_code":401}`)}
	report := "| Item | Source |\n|---|---|\n| GPT-6 | https://openai.com/blog/gpt-6 |\n"
	d, entry := newNotifyDispatcher(t, &scriptedExecutor{report: report}, &fakeNotifier{err: rejected}, &models.NotifyConfig{Connector: "tg"})
	if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
		t.Fatal(err)
	}
	want := "report not delivered via tg: telegram rejected the bot token (HTTP 401)"
	if run := lastRun(t, d); run == nil || len(run.Warnings) != 1 || run.Warnings[0] != want {
		t.Fatalf("warnings = %v, want [%q]", run.Warnings, want)
	}
}
