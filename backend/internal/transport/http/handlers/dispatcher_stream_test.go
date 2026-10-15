package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type streamDispatcher struct {
	testDispatcher
	bus *eventbus.Bus
}

func (s *streamDispatcher) Events() *eventbus.Bus { return s.bus }

func streamRequest(t *testing.T, h *DispatcherHandlers, query string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/admin/api/dispatcher/workspaces/ws/live"+query, nil).WithContext(ctx)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.StreamWorkspaceEvents(rr, req); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	return rr
}

// An automation's console asks for its own automation's stream and gets that run
// only: the heartbeat's report never appears on the daily news page.
func TestStreamWorkspaceEvents_AutomationStreamIsPerAutomation(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Stop()
	bus.Publish("ws", assistant.AgentEvent{ID: "news-1", Type: assistant.EventMessage, Channel: assistant.ChannelAutomation, Automation: "daily-llm-news"})
	bus.Publish("ws", assistant.AgentEvent{ID: "heartbeat-1", Type: assistant.EventMessage, Channel: assistant.ChannelAutomation, Automation: "heartbeat"})
	h := NewDispatcherHandlers(&streamDispatcher{bus: bus}, nil, logging.NewNopLogger())

	body := streamRequest(t, h, "?channel=automation&automation=daily-llm-news").Body.String()
	if !strings.Contains(body, "news-1") {
		t.Errorf("the automation's own event is missing: %s", body)
	}
	if strings.Contains(body, "heartbeat-1") {
		t.Errorf("another automation's event reached this stream: %s", body)
	}
}

func TestStreamWorkspaceEvents_AutomationStreamNeedsTheAutomation(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Stop()
	h := NewDispatcherHandlers(&streamDispatcher{bus: bus}, nil, logging.NewNopLogger())
	for _, query := range []string{"", "?channel=automation"} {
		if got := streamRequest(t, h, query).Code; got != http.StatusBadRequest {
			t.Errorf("%q: status %d, want 400", query, got)
		}
	}
}

func TestStreamWorkspaceEvents_AssistantChannelIsUnchanged(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Stop()
	bus.Publish("ws", assistant.AgentEvent{ID: "chat-1", Type: assistant.EventMessage, Channel: assistant.ChannelAssistant, ConversationID: "c"})
	h := NewDispatcherHandlers(&streamDispatcher{bus: bus}, nil, logging.NewNopLogger())
	if body := streamRequest(t, h, "?channel=assistant").Body.String(); !strings.Contains(body, "chat-1") {
		t.Errorf("the assistant stream lost its event: %s", body)
	}
}

// Clearing an automation's runs also clears the steps its console would replay:
// the confirmation says the run's events are removed.
func TestDeleteAutomationRuns_ClearsTheConsoleReplay(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	bus := eventbus.NewBus()
	defer bus.Stop()
	bus.Publish("ws", assistant.AgentEvent{ID: "n1", Type: assistant.EventMessage, Channel: assistant.ChannelAutomation, Automation: "news"})
	bus.Publish("ws", assistant.AgentEvent{ID: "h1", Type: assistant.EventMessage, Channel: assistant.ChannelAutomation, Automation: "heartbeat"})
	h := NewDispatcherHandlers(&streamDispatcher{bus: bus}, NewWorkspaceService(mgr), logging.NewNopLogger())

	req := httptest.NewRequest("DELETE", "/admin/api/dispatcher/runs/ws/news", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "news")
	rr := httptest.NewRecorder()
	h.DeleteAutomationRuns(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}

	if _, replay := bus.Subscribe("ws", assistant.AutomationStream("news")); len(replay) != 0 {
		t.Errorf("news replay still holds %d events", len(replay))
	}
	if _, replay := bus.Subscribe("ws", assistant.AutomationStream("heartbeat")); len(replay) != 1 {
		t.Errorf("the heartbeat's replay was touched: %d events", len(replay))
	}
}
