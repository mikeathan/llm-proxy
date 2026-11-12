package agentwatch

import (
	"context"
	"encoding/json"
	"fmt"

	"llm-proxy-stacks/sdk"
)

// plainOr returns a workspace id that is a plain name, or a fixed word.
func plainOr(workspace string) string {
	if plainName.MatchString(workspace) {
		return workspace
	}
	return "unnamed"
}

// Words is the sentence the operator is sent for an alert the stack published, in the core's own words (ADR-0036): the stack names an id and
// the vetted tokens, and the sentence is built here from a fixed table. It reports false for an id it does not know, and for a token that is
// not a plain one, so nothing the stack kept can put its own text in front of the operator.
func Words(a stacksdk.Alert) (string, bool) {
	switch a.ID {
	case "agent_tool_refused":
		if !plainName.MatchString(a.Automation) || (!plainTool.MatchString(a.Tool) && a.Tool != unknownTool) {
			return "", false
		}
		return fmt.Sprintf("Agent watch: the automation %q in workspace %q tried a tool call the safety rules refused (%s). Open its last run to see what it was doing.",
			a.Automation, plainOr(a.Workspace), a.Tool), true
	case "agent_new_destination":
		if !plainName.MatchString(a.Automation) || !plainHost.MatchString(a.Host) {
			return "", false
		}
		return fmt.Sprintf("Agent watch: the automation %q in workspace %q fetched from %s, a host it has not used before. If you did not change its task, open its last run to see why.",
			a.Automation, plainOr(a.Workspace), a.Host), true
	case "agent_secrets_found":
		if a.Count < 1 {
			return "", false
		}
		places := "1 file written by agents holds"
		if a.Count != 1 {
			places = fmt.Sprintf("%d files written by agents hold", a.Count)
		}
		return fmt.Sprintf("Agent watch: %s what looks like a key or token, in workspace %q. Open Settings, Safety to see where, then delete it and replace the key.",
			places, plainOr(a.Workspace)), true
	}
	return "", false
}

// AlertIDs are the alert ids Words has a sentence for, so a test can hold the table to the stack's list.
func AlertIDs() []string {
	return []string{"agent_tool_refused", "agent_new_destination", "agent_secrets_found"}
}

// Alerts is the stack's Events port: it takes an alert the stack published, words it and sends it to the operator. Notify sends a message to the
// operator's connectors (through the alert log); OnError hears an alert that could not be worded or sent.
type Alerts struct {
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
}

// Publish words and sends one alert. An event that is not an alert, or an alert that cannot be worded, is dropped and reported.
func (a Alerts) Publish(ctx context.Context, e stacksdk.Event) error {
	var alert stacksdk.Alert
	if e.Type != stacksdk.EventAlert || json.Unmarshal(e.Data, &alert) != nil {
		return a.fail(stacksdk.Errorf(stacksdk.CodeInvalid, "not an alert"))
	}
	message, ok := Words(alert)
	if !ok {
		return a.fail(stacksdk.Errorf(stacksdk.CodeInvalid, "an alert the service has no words for"))
	}
	if a.Notify == nil {
		return nil
	}
	if err := a.Notify(ctx, message); err != nil {
		return a.fail(stacksdk.Errorf(stacksdk.CodeInternal, "the alert could not be sent"))
	}
	return nil
}

func (a Alerts) fail(err *stacksdk.Error) error {
	if a.OnError != nil {
		a.OnError(err)
	}
	return err
}
