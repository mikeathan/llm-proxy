package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/models"
)

// refusing is a pre-flight that refuses with a coded reason until fixed is set.
type refusing struct {
	code, message string
	fixed         bool
	checked       int
}

func (r *refusing) check(_ context.Context, _ *AutomationEntry) error {
	r.checked++
	if r.fixed {
		return nil
	}
	return &PreflightError{Code: r.code, Message: r.message}
}

func TestPreflight_RefusalFailsTheRunWithoutStartingTheExecutor(t *testing.T) {
	exec := &scriptedExecutor{report: "ok"}
	n := &fakeNotifier{}
	d, entry := newNotifyDispatcher(t, exec, n, &models.NotifyConfig{Connector: "tg"})
	pf := &refusing{code: "scope_empty", message: "No network has been allowed for scanning."}
	d.preflight = pf.check

	err := d.executeAutomation(context.Background(), entry, "")
	var refusal *PreflightError
	if !errors.As(err, &refusal) || refusal.Code != "scope_empty" {
		t.Fatalf("err = %v, want the pre-flight refusal", err)
	}
	if len(exec.reqs) != 0 {
		t.Errorf("the executor ran %d time(s); a refused run must not wake the model", len(exec.reqs))
	}
	state, _ := d.persistence.ReadState("ws")
	if len(state.History) != 1 || !strings.Contains(state.History[0].Error, "No network has been allowed") {
		t.Errorf("the refusal must be the run's recorded failure: %+v", state.History)
	}
	if state.IsRunning() {
		t.Error("the workspace must not be left marked running")
	}
}

// notices are the "did not run" messages among everything the notifier delivered (a successful run delivers its report).
func notices(n *fakeNotifier) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, m := range n.sent {
		if strings.Contains(m, "did not run") {
			out = append(out, m)
		}
	}
	return out
}

func TestPreflight_TheOperatorIsToldOncePerReasonUntilTheAutomationSucceeds(t *testing.T) {
	exec := &scriptedExecutor{report: "ok"}
	n := &fakeNotifier{}
	d, entry := newNotifyDispatcher(t, exec, n, &models.NotifyConfig{Connector: "tg"})
	pf := &refusing{code: "scope_empty", message: "No network has been allowed for scanning."}
	d.preflight = pf.check
	run := func() { _ = d.executeAutomation(context.Background(), entry, "") }

	run()
	run()
	run()
	if got := notices(n); len(got) != 1 || !strings.Contains(got[0], "No network has been allowed") {
		t.Fatalf("three blocked nights must send one notice, sent %q", got)
	}

	pf.code, pf.message = "nmap_missing", "nmap is not available."
	run()
	if got := notices(n); len(got) != 2 {
		t.Fatalf("a different reason is news: %q", got)
	}

	pf.fixed = true
	run() // succeeds and re-arms
	pf.fixed, pf.code, pf.message = false, "scope_empty", "No network has been allowed for scanning."
	run()
	if got := notices(n); len(got) != 3 {
		t.Fatalf("after recovery the same reason is news again: %q", got)
	}
}

func TestPreflight_ANilCheckRunsEverything(t *testing.T) {
	exec := &scriptedExecutor{report: "ok"}
	d, entry := newNotifyDispatcher(t, exec, &fakeNotifier{}, nil)
	if err := d.executeAutomation(context.Background(), entry, ""); err != nil || len(exec.reqs) != 1 {
		t.Errorf("err = %v, calls = %d", err, len(exec.reqs))
	}
}

func TestReasonNoticeLimiter(t *testing.T) {
	var l reasonNoticeLimiter
	if !l.reserve("ws/a", "x") || l.reserve("ws/a", "x") {
		t.Error("the first reservation passes, the repeat does not")
	}
	if !l.reserve("ws/a", "y") || !l.reserve("ws/b", "x") {
		t.Error("another reason, or another automation, is separate")
	}
	l.release("ws/a", "x")
	if !l.reserve("ws/a", "x") {
		t.Error("a released reservation may be taken again")
	}
	l.reset("ws/a")
	if !l.reserve("ws/a", "x") || !l.reserve("ws/a", "y") {
		t.Error("reset forgets every reason of the automation")
	}
	if l.reserve("ws/b", "x") {
		t.Error("reset must not touch another automation")
	}
}
