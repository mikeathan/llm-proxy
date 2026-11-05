package agentwatch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

type memStore struct {
	mu  sync.Mutex
	rec models.AgentWatch
	err error
}

func (m *memStore) Read() (models.AgentWatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rec, m.err
}

func (m *memStore) Write(r models.AgentWatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = r
	return nil
}

type sent struct {
	mu   sync.Mutex
	msgs []string
}

func (s *sent) notify(_ context.Context, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *sent) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.msgs...)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestWatch() (*Watch, *memStore, *sent, *clock) {
	store, out := &memStore{}, &sent{}
	c := &clock{t: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}
	return New(Deps{Store: store, Notify: out.notify, Now: c.now}), store, out, c
}

func refused(automation, tool, errText string) assistant.AgentEvent {
	return assistant.AgentEvent{Type: assistant.EventGuardrailViolation, Channel: assistant.ChannelAutomation, Automation: automation,
		Payload: map[string]string{"tool": tool, "error": errText}}
}

func fetch(automation, url string) assistant.AgentEvent {
	return assistant.AgentEvent{Type: assistant.EventToolCall, Channel: assistant.ChannelAutomation, Automation: automation,
		Payload: models.ToolCall{Function: models.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"` + url + `"}`}}}
}

func TestWatch_ARefusalInAnAutomationIsRecordedAndAlertedOnceADay(t *testing.T) {
	w, store, out, c := newTestWatch()
	ctx := t.Context()
	w.handle(ctx, "ws", refused("nightly", "read_file", "path /etc/shadow is outside the workspace"))
	w.handle(ctx, "ws", refused("nightly", "read_file", "again"))
	if got := store.rec.Events; len(got) != 2 || got[0].Kind != models.AgentToolRefused || got[0].Tool != "read_file" || got[0].Automation != "nightly" {
		t.Fatalf("events = %+v", got)
	}
	for _, e := range store.rec.Events {
		if strings.Contains(e.Tool+e.Host+e.Where+e.Detail, "shadow") {
			t.Fatalf("an error text reached the record: %+v", e)
		}
	}
	if msgs := out.all(); len(msgs) != 1 || !strings.Contains(msgs[0], `"nightly"`) || !strings.Contains(msgs[0], "read_file") {
		t.Fatalf("alerts = %q; want one naming the automation and tool", msgs)
	}
	c.t = c.t.Add(25 * time.Hour)
	w.handle(ctx, "ws", refused("nightly", "read_file", "x"))
	if len(out.all()) != 2 {
		t.Error("a refusal on a later day alerts again")
	}
}

func TestWatch_ChatRefusalsAndApprovalsAreRecordedNotAlerted(t *testing.T) {
	w, store, out, _ := newTestWatch()
	chat := refused("", "execute_terminal_command", "blocked")
	chat.Channel = assistant.ChannelAssistant
	w.handle(t.Context(), "ws", chat)
	w.handle(t.Context(), "ws", assistant.AgentEvent{Type: assistant.EventGuardrailBlocked, Channel: assistant.ChannelAutomation, Automation: "nightly",
		Payload: assistant.GuardrailBlockedPayload{Tool: "write_file", Args: `{"path":"secret"}`}})
	if len(store.rec.Events) != 2 || store.rec.Events[1].Kind != models.AgentApprovalAsked || store.rec.Events[1].Tool != "write_file" {
		t.Fatalf("events = %+v", store.rec.Events)
	}
	if len(out.all()) != 0 {
		t.Errorf("alerts = %q; a person is at the chat, and an approval is a question already asked", out.all())
	}
}

func TestWatch_DestinationsAreLearnedForAWeekThenANewHostIsReported(t *testing.T) {
	w, store, out, c := newTestWatch()
	ctx := t.Context()
	w.handle(ctx, "ws", fetch("weather", "https://API.weather.example/v1/today?city=x&key=SECRET"))
	c.t = c.t.Add(6 * 24 * time.Hour)
	w.handle(ctx, "ws", fetch("weather", "https://cdn.weather.example/map.png"))
	if len(store.rec.Events) != 0 || len(out.all()) != 0 {
		t.Fatalf("the learning week records hosts only: %+v %q", store.rec.Events, out.all())
	}
	d := store.rec.Destinations["ws/weather"]
	if len(d.Hosts) != 2 || d.Hosts[0] != "api.weather.example" {
		t.Fatalf("destinations = %+v; want two lower-cased host names, no path or query", d)
	}
	c.t = c.t.Add(2 * 24 * time.Hour)
	w.handle(ctx, "ws", fetch("weather", "https://api.weather.example/v1/today"))
	w.handle(ctx, "ws", fetch("weather", "http://203.0.113.9:8080/collect?d=abc"))
	if got := store.rec.Events; len(got) != 1 || got[0].Kind != models.AgentNewDestination || got[0].Host != "203.0.113.9" {
		t.Fatalf("events = %+v; want one new destination with the host only", got)
	}
	if msgs := out.all(); len(msgs) != 1 || !strings.Contains(msgs[0], "203.0.113.9") || strings.Contains(msgs[0], "collect") {
		t.Fatalf("alerts = %q", msgs)
	}
	w.handle(ctx, "ws", fetch("weather", "http://203.0.113.9/again"))
	if len(store.rec.Events) != 1 {
		t.Error("a host is new once")
	}
}

func TestWatch_AnAutomationThatBrowsesManyHostsOnlyRecords(t *testing.T) {
	w, store, out, c := newTestWatch()
	ctx := t.Context()
	for _, h := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		w.handle(ctx, "ws", fetch("news", "https://"+h+".news.example/story"))
	}
	if !store.rec.Destinations["ws/news"].Many {
		t.Fatal("nine hosts in the learning week marks the automation as one that browses")
	}
	c.t = c.t.Add(8 * 24 * time.Hour)
	w.handle(ctx, "ws", fetch("news", "https://z.news.example/story"))
	if len(store.rec.Events) != 1 || len(out.all()) != 0 {
		t.Errorf("events %+v alerts %q; a browsing automation's new host is recorded, never alerted", store.rec.Events, out.all())
	}
}

func TestWatch_UnreadableRecordNeverCrashesAndBadInputIsDropped(t *testing.T) {
	w, store, out, _ := newTestWatch()
	store.err = errors.New("corrupt")
	w.handle(t.Context(), "ws", refused("nightly", "read_file", "x"))
	if len(out.all()) != 0 {
		t.Error("nothing is alerted from a record that could not be read")
	}
	store.err = nil
	w.handle(t.Context(), "ws", refused("nightly", "rm -rf /;", "x"))
	w.handle(t.Context(), "ws", fetch("nightly", "javascript:alert(1)"))
	w.handle(t.Context(), "ws", fetch("nightly", "https://exa mple.com/"))
	for _, e := range store.rec.Events {
		if e.Tool != "" && e.Tool != unknownTool {
			t.Errorf("an unvetted tool name was kept: %+v", e)
		}
	}
	if len(store.rec.Destinations) != 0 {
		t.Errorf("hosts that are not plain names were kept: %+v", store.rec.Destinations)
	}
}

func TestWatch_ObserveNeverBlocksAndRunDrains(t *testing.T) {
	w, store, _, _ := newTestWatch()
	for range queueSize + 10 {
		w.Observe("ws", refused("nightly", "read_file", "x"))
	}
	if w.Dropped() != 10 {
		t.Errorf("dropped = %d, want 10", w.Dropped())
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rec, _ := store.Read()
		if len(rec.Events) == maxEvents || len(rec.Events) == queueSize || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if rec, _ := store.Read(); len(rec.Events) > maxEvents || len(rec.Events) == 0 {
		t.Errorf("events = %d; want the queue drained within the cap", len(rec.Events))
	}
}
