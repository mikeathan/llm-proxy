// Package stackalerts is the Events port a hosted stack publishes its alerts through: the core words each alert from a fixed table of its own
// (ADR-0036) and sends the sentence to the operator, so nothing a stack kept can put its own text in front of anyone.
package stackalerts

import (
	"context"
	"encoding/json"

	"llm-proxy-stacks/sdk"
)

// Sink takes the alerts a stack published. Words turns one into a sentence, or reports false for an id or a token it does not know (the alert
// is then dropped and reported); Notify delivers the sentence; OnError hears an alert that could not be worded or delivered.
type Sink struct {
	Words   func(stacksdk.Alert) (string, bool)
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
}

// Publish words and sends one alert. An event that is not an alert, or an alert that cannot be worded, is dropped and reported.
func (s Sink) Publish(ctx context.Context, e stacksdk.Event) error {
	var alert stacksdk.Alert
	if e.Type != stacksdk.EventAlert || json.Unmarshal(e.Data, &alert) != nil {
		return s.fail(stacksdk.Errorf(stacksdk.CodeInvalid, "not an alert"))
	}
	message, ok := "", false
	if s.Words != nil {
		message, ok = s.Words(alert)
	}
	if !ok {
		return s.fail(stacksdk.Errorf(stacksdk.CodeInvalid, "an alert the service has no words for"))
	}
	if s.Notify == nil {
		return nil
	}
	if err := s.Notify(ctx, message); err != nil {
		return s.fail(stacksdk.Errorf(stacksdk.CodeInternal, "the alert could not be sent"))
	}
	return nil
}

func (s Sink) fail(err *stacksdk.Error) error {
	if s.OnError != nil {
		s.OnError(err)
	}
	return err
}
