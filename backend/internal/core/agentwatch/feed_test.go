package agentwatch

import (
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

func refused(automation, tool string) assistant.AgentEvent {
	return assistant.AgentEvent{Type: assistant.EventGuardrailViolation, Channel: assistant.ChannelAutomation, Automation: automation,
		Payload: map[string]string{"tool": tool, "error": "path /etc/shadow is outside the workspace"}}
}

func fetch(automation, url string) assistant.AgentEvent {
	return assistant.AgentEvent{Type: assistant.EventToolCall, Channel: assistant.ChannelAutomation, Automation: automation,
		Payload: models.ToolCall{Function: models.FunctionCall{Name: models.ToolNetworkFetch, Arguments: `{"url":"` + url + `"}`}}}
}

func observed(t *testing.T, ev assistant.AgentEvent) (stacksdk.Observation, bool) {
	t.Helper()
	var got []stacksdk.Observation
	Feed{Push: func(o stacksdk.Observation) { got = append(got, o) }}.Observe("ws", ev)
	if len(got) > 1 {
		t.Fatalf("one event pushed %d observations", len(got))
	}
	if len(got) == 0 {
		return stacksdk.Observation{}, false
	}
	return got[0], true
}

func TestFeed_ARefusalInAnAutomationIsUnattendedAndCarriesNoErrorText(t *testing.T) {
	o, ok := observed(t, refused("nightly", "read_file"))
	want := stacksdk.Observation{Kind: stacksdk.ObservedRefused, Workspace: "ws", Automation: "nightly", Unattended: true, Tool: "read_file"}
	if !ok || o != want {
		t.Fatalf("observation = %+v, want %+v", o, want)
	}
}

func TestFeed_AChatRefusalIsNotUnattended(t *testing.T) {
	ev := refused("", "execute_terminal_command")
	ev.Channel = assistant.ChannelAssistant
	o, ok := observed(t, ev)
	if !ok || o.Unattended || o.Automation != "" || o.Kind != stacksdk.ObservedRefused {
		t.Fatalf("observation = %+v", o)
	}
}

func TestFeed_AnApprovalCarriesTheToolNameAndNoArguments(t *testing.T) {
	o, ok := observed(t, assistant.AgentEvent{Type: assistant.EventGuardrailBlocked, Channel: assistant.ChannelAutomation, Automation: "nightly",
		Payload: assistant.GuardrailBlockedPayload{Tool: "write_file", Args: `{"path":"secret"}`}})
	want := stacksdk.Observation{Kind: stacksdk.ObservedApproval, Workspace: "ws", Automation: "nightly", Unattended: true, Tool: "write_file"}
	if !ok || o != want {
		t.Fatalf("observation = %+v, want %+v", o, want)
	}
}

func TestFeed_AFetchCarriesTheLowerCasedHostAndNothingElse(t *testing.T) {
	o, ok := observed(t, fetch("weather", "https://API.weather.example/v1/today?city=x&key=SECRET"))
	want := stacksdk.Observation{Kind: stacksdk.ObservedFetch, Workspace: "ws", Automation: "weather", Unattended: true, Host: "api.weather.example"}
	if !ok || o != want {
		t.Fatalf("observation = %+v, want %+v", o, want)
	}
	if o, ok := observed(t, fetch("weather", "http://203.0.113.9:8080/collect?d=abc")); !ok || o.Host != "203.0.113.9" {
		t.Fatalf("observation = %+v; a port, path and query are dropped", o)
	}
}

func TestFeed_BadInputIsDroppedOrBecomesAFixedWord(t *testing.T) {
	if o, ok := observed(t, refused("nightly", "rm -rf /;")); !ok || o.Tool != unknownTool {
		t.Errorf("an unvetted tool name must become %q: %+v", unknownTool, o)
	}
	if o, ok := observed(t, refused("a name; with <junk>", "read_file")); !ok || o.Automation != "" {
		t.Errorf("an unvetted automation name is not passed on: %+v", o)
	}
	for _, url := range []string{"javascript:alert(1)", "https://exa mple.com/", "ftp://a.example/x", "not a url", ""} {
		if _, ok := observed(t, fetch("nightly", url)); ok {
			t.Errorf("a fetch of %q must push nothing", url)
		}
	}
}

func TestFeed_OnlyTheEventsTheRecordUsesArePushed(t *testing.T) {
	for _, ev := range []assistant.AgentEvent{
		{Type: assistant.EventToolCall, Channel: assistant.ChannelAssistant, Automation: "nightly", Payload: fetch("nightly", "https://a.example/").Payload}, // a chat's fetch
		{Type: assistant.EventToolCall, Channel: assistant.ChannelAutomation, Automation: "nightly", Payload: models.ToolCall{Function: models.FunctionCall{Name: "read_file", Arguments: `{"url":"https://a.example/"}`}}},
		{Type: assistant.EventGuardrailBlocked, Channel: assistant.ChannelAutomation, Automation: "nightly", Payload: "not the payload"},
		{Type: "something_else", Channel: assistant.ChannelAutomation, Automation: "nightly"},
		fetch("", "https://a.example/"), // no automation to learn
	} {
		if o, ok := observed(t, ev); ok {
			t.Errorf("event %+v pushed %+v", ev.Type, o)
		}
	}
}

func TestFeed_TheSwitchIsReadOnEveryEventAndNilMeansOnAndNoPushIsSafe(t *testing.T) {
	on, pushed := false, 0
	f := Feed{Push: func(stacksdk.Observation) { pushed++ }, ActivityOn: func() bool { return on }}
	f.Observe("ws", refused("nightly", "read_file"))
	if pushed != 0 {
		t.Fatalf("pushed %d while the record is off", pushed)
	}
	on = true
	f.Observe("ws", refused("nightly", "read_file"))
	if pushed != 1 {
		t.Fatalf("pushed %d after switching on, want 1 (the switch is read on every event)", pushed)
	}
	Feed{Push: func(stacksdk.Observation) { pushed++ }}.Observe("ws", refused("nightly", "read_file"))
	if pushed != 2 {
		t.Errorf("a feed with no switch is on, pushed = %d", pushed)
	}
	Feed{}.Observe("ws", refused("nightly", "read_file")) // no receiver: nothing to do, no crash
}
