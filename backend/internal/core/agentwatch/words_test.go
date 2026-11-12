package agentwatch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// The sentences below are the ones the service sent before the stack existed (captured from that code, 2026-11-12). Words is the only place
// they are built now, so any change to one is a deliberate change here.
func TestWords_AreTheSentencesTheServiceAlwaysSent(t *testing.T) {
	for name, tc := range map[string]struct {
		alert stacksdk.Alert
		want  string
	}{
		"a refusal": {stacksdk.Alert{ID: "agent_tool_refused", Workspace: "ws", Automation: "nightly", Tool: "read_file"},
			`Agent watch: the automation "nightly" in workspace "ws" tried a tool call the safety rules refused (read_file). Open its last run to see what it was doing.`},
		"a refusal in a workspace with an odd id": {stacksdk.Alert{ID: "agent_tool_refused", Workspace: "bad ws!", Automation: "nightly", Tool: "read_file"},
			`Agent watch: the automation "nightly" in workspace "unnamed" tried a tool call the safety rules refused (read_file). Open its last run to see what it was doing.`},
		"a refusal of an unnamed tool": {stacksdk.Alert{ID: "agent_tool_refused", Workspace: "ws", Automation: "nightly", Tool: "another tool"},
			`Agent watch: the automation "nightly" in workspace "ws" tried a tool call the safety rules refused (another tool). Open its last run to see what it was doing.`},
		"a new destination": {stacksdk.Alert{ID: "agent_new_destination", Workspace: "ws", Automation: "weather", Host: "b.example"},
			`Agent watch: the automation "weather" in workspace "ws" fetched from b.example, a host it has not used before. If you did not change its task, open its last run to see why.`},
		"one secret": {stacksdk.Alert{ID: "agent_secrets_found", Workspace: "ws", Count: 1},
			`Agent watch: 1 file written by agents holds what looks like a key or token, in workspace "ws". Open Settings, Safety to see where, then delete it and replace the key.`},
		"two secrets": {stacksdk.Alert{ID: "agent_secrets_found", Workspace: "x y", Count: 2},
			`Agent watch: 2 files written by agents hold what looks like a key or token, in workspace "x y". Open Settings, Safety to see where, then delete it and replace the key.`},
	} {
		if got, ok := Words(tc.alert); !ok || got != tc.want {
			t.Errorf("%s:\n got  %q (%v)\n want %q", name, got, ok, tc.want)
		}
	}
}

func TestWords_RefuseWhatTheyHaveNoSentenceForOrATokenThatIsNotPlain(t *testing.T) {
	for name, a := range map[string]stacksdk.Alert{
		"an id the table does not know":      {ID: "agent_invented", Workspace: "ws"},
		"an empty id":                        {},
		"an automation with junk":            {ID: "agent_tool_refused", Automation: `x"; drop`, Tool: "read_file"},
		"a tool that is not a plain token":   {ID: "agent_tool_refused", Automation: "n", Tool: "rm -rf /;"},
		"a host that is not a plain name":    {ID: "agent_new_destination", Automation: "n", Host: "exa mple.com"},
		"a secrets alert with nothing found": {ID: "agent_secrets_found", Workspace: "ws"},
	} {
		if got, ok := Words(a); ok {
			t.Errorf("%s: worded as %q", name, got)
		}
	}
}

func publish(a Alerts, v any) error {
	data, _ := json.Marshal(v)
	return a.Publish(context.Background(), stacksdk.Event{Type: stacksdk.EventAlert, Subject: "x", Data: data, At: time.Now()})
}

func TestAlerts_WordAndSendAndDropWhatTheyCannotWord(t *testing.T) {
	var sent []string
	var heard []error
	a := Alerts{Notify: func(_ context.Context, m string) error { sent = append(sent, m); return nil }, OnError: func(e error) { heard = append(heard, e) }}
	if err := publish(a, stacksdk.Alert{ID: "agent_secrets_found", Workspace: "ws", Count: 1}); err != nil || len(sent) != 1 {
		t.Fatalf("err %v sent %q", err, sent)
	}
	if err := publish(a, stacksdk.Alert{ID: "agent_invented"}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid || len(sent) != 1 || len(heard) != 1 {
		t.Errorf("an alert with no words: err %v sent %q heard %v", err, sent, heard)
	}
	if err := a.Publish(context.Background(), stacksdk.Event{Type: "other", Data: json.RawMessage(`{}`)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("an event that is not an alert: %v", err)
	}
	if err := a.Publish(context.Background(), stacksdk.Event{Type: stacksdk.EventAlert, Data: json.RawMessage(`[1]`)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("a body that is not an alert: %v", err)
	}
}

func TestAlerts_AFailedSendIsAFixedSentenceNotTheConnectorsWords(t *testing.T) {
	a := Alerts{Notify: func(context.Context, string) error { return errors.New("telegram token 123:abc rejected") }}
	err := publish(a, stacksdk.Alert{ID: "agent_secrets_found", Workspace: "ws", Count: 1})
	if stacksdk.CodeOf(err) != stacksdk.CodeInternal || err.Error() != "internal: the alert could not be sent" {
		t.Errorf("err = %v", err)
	}
	if err := publish(Alerts{}, stacksdk.Alert{ID: "agent_secrets_found", Workspace: "ws", Count: 1}); err != nil {
		t.Errorf("with no way to notify the alert is dropped quietly: %v", err)
	}
}
