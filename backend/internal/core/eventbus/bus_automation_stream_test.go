package eventbus

import (
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
)

func automationEvent(automation, id string) assistant.AgentEvent {
	return assistant.AgentEvent{ID: id, Type: assistant.EventMessage, Channel: assistant.ChannelAutomation, Automation: automation}
}

func ids(events []assistant.AgentEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.ID
	}
	return out
}

// Automation events are routed by the automation that produced them, so one
// automation's console never shows another's run (the heartbeat's report once
// appeared on the daily news page).
func TestAutomationEvents_AreRoutedByTheirAutomation(t *testing.T) {
	bus := NewBus()
	defer bus.Stop()

	news, _ := bus.Subscribe("ws", assistant.AutomationStream("news"))
	bus.Publish("ws", automationEvent("news", "n1"))
	bus.Publish("ws", automationEvent("heartbeat", "h1"))
	bus.Publish("ws", automationEvent("news", "n2"))

	var live []string
	for i := 0; i < 2; i++ {
		select {
		case ev := <-news:
			live = append(live, ev.ID)
		case <-time.After(time.Second):
			t.Fatalf("live stream delivered %v, want n1 n2", live)
		}
	}
	select {
	case ev := <-news:
		t.Errorf("another automation's event %q reached the stream", ev.ID)
	case <-time.After(50 * time.Millisecond):
	}

	_, replay := bus.Subscribe("ws", assistant.AutomationStream("news"))
	if got := ids(replay); len(got) != 2 || got[0] != "n1" || got[1] != "n2" {
		t.Errorf("replay = %v, want [n1 n2]", got)
	}
	_, other := bus.Subscribe("ws", assistant.AutomationStream("heartbeat"))
	if got := ids(other); len(got) != 1 || got[0] != "h1" {
		t.Errorf("heartbeat replay = %v, want [h1]", got)
	}
}

// A run's boot clears only that automation's earlier events: the news run's
// steps stay available while the heartbeat runs.
func TestClearingOneAutomation_KeepsTheOthers(t *testing.T) {
	bus := NewBus()
	defer bus.Stop()
	bus.Publish("ws", automationEvent("news", "n1"))
	bus.Publish("ws", automationEvent("heartbeat", "h1"))

	bus.Clear("ws", assistant.AutomationStream("heartbeat"))

	_, news := bus.Subscribe("ws", assistant.AutomationStream("news"))
	_, heartbeat := bus.Subscribe("ws", assistant.AutomationStream("heartbeat"))
	if len(news) != 1 || len(heartbeat) != 0 {
		t.Errorf("news replay %v, heartbeat replay %v: want the news event kept and the heartbeat's cleared", ids(news), ids(heartbeat))
	}
}

func TestAutomationStream_NamesTheAutomation(t *testing.T) {
	if assistant.AutomationStream("a") == assistant.AutomationStream("b") || assistant.AutomationStream("a") == assistant.ChannelAutomation {
		t.Error("each automation needs its own stream, distinct from the bare channel")
	}
}
