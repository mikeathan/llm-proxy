package stackalerts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"llm-proxy-stacks/sdk"
)

func event(t *testing.T, a stacksdk.Alert) stacksdk.Event {
	t.Helper()
	data, _ := json.Marshal(a)
	return stacksdk.Event{Type: stacksdk.EventAlert, Subject: a.ID, Data: data}
}

func TestASinkWordsAndSendsAnAlertAndDropsWhatItHasNoWordsFor(t *testing.T) {
	var sent []string
	var errs []error
	s := Sink{
		Words:   func(a stacksdk.Alert) (string, bool) { return "said " + a.ID, a.ID == "known" },
		Notify:  func(_ context.Context, m string) error { sent = append(sent, m); return nil },
		OnError: func(e error) { errs = append(errs, e) },
	}
	if err := s.Publish(context.Background(), event(t, stacksdk.Alert{ID: "known"})); err != nil || len(sent) != 1 || sent[0] != "said known" {
		t.Fatalf("%v %v", err, sent)
	}
	if err := s.Publish(context.Background(), event(t, stacksdk.Alert{ID: "other"})); err == nil || len(sent) != 1 || len(errs) != 1 {
		t.Errorf("an unknown alert must be dropped and reported: %v %v %v", err, sent, errs)
	}
	if err := s.Publish(context.Background(), stacksdk.Event{Type: "log"}); err == nil {
		t.Error("an event that is not an alert must be refused")
	}
}

func TestASinkNeverPutsTheSendersFailureInFrontOfTheCaller(t *testing.T) {
	s := Sink{
		Words:  func(stacksdk.Alert) (string, bool) { return "x", true },
		Notify: func(context.Context, string) error { return errors.New("token 123:abc rejected") },
	}
	err := s.Publish(context.Background(), event(t, stacksdk.Alert{ID: "a"}))
	if err == nil || err.Error() == "token 123:abc rejected" {
		t.Errorf("the failure's own text must not pass through: %v", err)
	}
}
