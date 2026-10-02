package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

// failingExecutor always fails a run with a fixed error, appending the failed
// tail history entry the real executor writes before returning.
type failingExecutor struct{ err error }

func (e *failingExecutor) Execute(_ context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	if req.State != nil {
		req.State.History = append(req.State.History, models.AutomationRun{
			WorkspaceID:    req.WorkspaceID,
			AutomationName: req.AutomationName,
			Error:          e.err.Error(),
		})
	}
	return nil, e.err
}

func (e *failingExecutor) ShellPGID(context.Context, string) (int, error) { return 0, nil }
func (e *failingExecutor) ModelTimeout(string) time.Duration              { return 0 }

// TestFailRun_PublishesErrorPayloadForSharedRenderer guards the automation
// error event contract: the shared frontend consumer (messageBuilder) reads
// {error, hint} and falls back to "Unknown error" otherwise. Publishing a
// proxy.Message here made every automation failure render as "Unknown error"
// while the classified cause sat unused in the payload.
func TestFailRun_PublishesErrorPayloadForSharedRenderer(t *testing.T) {
	connErr := errors.New(`Post "http://0.0.0.0:8081/v1/chat/completions": dial tcp 0.0.0.0:8081: connect: connection refused`)
	d := newLaneDispatcher(t, &failingExecutor{err: connErr})
	registerManualAutomation(t, d, "ws", "a")

	events, _ := d.Events().Subscribe("ws", assistant.ChannelAutomation)
	defer d.Events().Unsubscribe("ws", assistant.ChannelAutomation, events)

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type != assistant.EventError {
				continue
			}
			payload, ok := ev.Payload.(map[string]string)
			if !ok {
				t.Fatalf("error payload type = %T, want map[string]string {error,hint}", ev.Payload)
			}
			if !strings.Contains(payload["error"], "model server") {
				t.Errorf("error payload = %q, want the classified connection-refused summary", payload["error"])
			}
			return
		case <-deadline:
			t.Fatal("no EventError published")
		}
	}
}

// TestExecute_PrepareFailureIsSurfaced guards the silent-trigger bug: a run that
// fails before the executor starts (here, the task file is missing) used to be
// dropped by the run lane — no log line, no event — so "Run now" looked like it
// did nothing. The failure must reach the automation event channel.
func TestExecute_PrepareFailureIsSurfaced(t *testing.T) {
	d := newLaneDispatcher(t, &mockExecutor{})
	registerManualAutomation(t, d, "ws", "a")
	if err := d.persistence.DeleteTaskFile("ws", "task.md"); err != nil {
		t.Fatal(err)
	}

	events, _ := d.Events().Subscribe("ws", assistant.ChannelAutomation)
	defer d.Events().Unsubscribe("ws", assistant.ChannelAutomation, events)

	if _, err := d.Trigger("ws", "a", ""); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type != assistant.EventError {
				continue
			}
			payload, ok := ev.Payload.(map[string]string)
			if !ok || !strings.Contains(payload["error"], "task.md") {
				t.Fatalf("error payload = %#v, want it to name the task file", ev.Payload)
			}
			assertFailureRecorded(t, d, "ws", "a", "task.md")
			return
		case <-deadline:
			t.Fatal("prepare failure was dropped: no EventError published")
		}
	}
}

// assertFailureRecorded checks the persisted side of a setup failure: the UI's
// "Last result" card reads LastRuns, so the failure must outlive the event.
func assertFailureRecorded(t *testing.T, d *Dispatcher, ws, name, wantInErr string) {
	t.Helper()
	state, err := d.persistence.ReadState(ws)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	last, ok := state.LastRuns[name]
	if !ok || !strings.Contains(last.Error, wantInErr) {
		t.Fatalf("LastRuns[%q] = %+v, want an error naming %q", name, last, wantInErr)
	}
	if n := len(state.History); n != 1 || state.History[n-1].Error != last.Error {
		t.Errorf("history = %+v, want the failed run as its only entry", state.History)
	}
	if state.IsRunning() {
		t.Error("workspace left marked running after a setup failure")
	}
}

func TestNewExecuteRequest_CarriesPerRunSettings(t *testing.T) {
	entry := &AutomationEntry{
		Workspace:    "ws",
		Name:         "nightly",
		TaskFile:     "task.md",
		Model:        "m",
		LoopStrategy: models.LoopStrategyReact,
		AllowedTools: []string{"read_file"},
		RecordingRef: "rec-1",
		NetworkGrant: models.NetworkScopeLan,
		MemoryMode:   models.MemoryModeHot,
	}
	req := newExecuteRequest(entry, nil, "do it", "")
	if req.MemoryMode != models.MemoryModeHot || req.NetworkGrant != models.NetworkScopeLan || req.LoopStrategy != "react" {
		t.Errorf("per-run settings dropped: %+v", req)
	}
	if req.RecordingRef != "rec-1" {
		t.Errorf("entry recording ref = %q, want rec-1", req.RecordingRef)
	}
	if got := newExecuteRequest(entry, nil, "do it", "override").RecordingRef; got != "override" {
		t.Errorf("an explicit override wins, got %q", got)
	}
}
