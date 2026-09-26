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
