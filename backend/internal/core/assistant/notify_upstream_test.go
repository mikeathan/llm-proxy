package assistant

import (
	"testing"

	"llm-proxy/internal/core/proxy"
)

// TestNotifyUpstream_Status verifies the retry observer maps a status retry to
// an EventUpstream with the correct wire payload.
func TestNotifyUpstream_Status(t *testing.T) {
	var events []AgentEvent
	agent := &Agent{
		deps: AgentRuntimeDeps{Observer: func(ev AgentEvent) { events = append(events, ev) }},
		config: AgentConfig{
			Channel:        ChannelAssistant,
			ConversationID: "conv_123",
		},
	}

	agent.notifyUpstream(proxy.RetryInfo{
		Reason:      proxy.RetryReasonStatus,
		Attempt:     2,
		MaxAttempts: 3,
		Status:      529,
		ElapsedMs:   1500,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != EventUpstream {
		t.Errorf("expected EventUpstream, got %q", ev.Type)
	}
	if ev.Channel != ChannelAssistant {
		t.Errorf("expected Channel assistant, got %q", ev.Channel)
	}
	if ev.ConversationID != "conv_123" {
		t.Errorf("expected ConversationID conv_123, got %q", ev.ConversationID)
	}
	p, ok := ev.Payload.(UpstreamEventPayload)
	if !ok {
		t.Fatalf("expected UpstreamEventPayload, got %T", ev.Payload)
	}
	if p.Event != "retry" {
		t.Errorf("expected event=retry, got %q", p.Event)
	}
	if p.Reason != "status" {
		t.Errorf("expected reason=status, got %q", p.Reason)
	}
	if p.Attempt != 2 {
		t.Errorf("expected attempt=2, got %d", p.Attempt)
	}
	if p.MaxAttempts != 3 {
		t.Errorf("expected max_attempts=3, got %d", p.MaxAttempts)
	}
	if p.Status != 529 {
		t.Errorf("expected status=529, got %d", p.Status)
	}
	if p.Error != "" {
		t.Errorf("expected empty error for status retry, got %q", p.Error)
	}
	if p.ElapsedMs != 1500 {
		t.Errorf("expected elapsed_ms=1500, got %d", p.ElapsedMs)
	}
}

// TestNotifyUpstream_Transport verifies a transport retry maps error text into
// the payload and leaves status zero.
func TestNotifyUpstream_Transport(t *testing.T) {
	var events []AgentEvent
	agent := &Agent{
		deps: AgentRuntimeDeps{Observer: func(ev AgentEvent) { events = append(events, ev) }},
		config: AgentConfig{
			Channel:        ChannelAutomation,
			ConversationID: "",
		},
	}

	agent.notifyUpstream(proxy.RetryInfo{
		Reason:      proxy.RetryReasonTransport,
		Attempt:     1,
		MaxAttempts: 3,
		Error:       "unexpected EOF",
		ErrClass:    "connection-closed",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	p, ok := events[0].Payload.(UpstreamEventPayload)
	if !ok {
		t.Fatalf("expected UpstreamEventPayload, got %T", events[0].Payload)
	}
	if p.Reason != "transport" {
		t.Errorf("expected reason=transport, got %q", p.Reason)
	}
	if p.Error != "unexpected EOF" {
		t.Errorf("expected error text, got %q", p.Error)
	}
	if p.ErrClass != "connection-closed" {
		t.Errorf("expected err_class=connection-closed, got %q", p.ErrClass)
	}
	if p.Status != 0 {
		t.Errorf("expected status=0 for transport retry, got %d", p.Status)
	}
	if events[0].Channel != ChannelAutomation {
		t.Errorf("expected Channel automation, got %q", events[0].Channel)
	}
}

// TestNotifyUpstream_NoObserver ensures the notify path is a no-op when no
// observer is wired (e.g. connection-test clients), so it never panics.
func TestNotifyUpstream_NoObserver(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{}}
	agent.notifyUpstream(proxy.RetryInfo{
		Reason:      proxy.RetryReasonStatus,
		Attempt:     1,
		MaxAttempts: 3,
		Status:      503,
	})
}
