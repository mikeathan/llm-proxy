package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"llm-proxy-stacks/radio"

	stacksdk "llm-proxy-stacks/sdk"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/radiowatch"
	"llm-proxy/internal/core/stackalerts"
	"llm-proxy/models"
)

type radioAlertStore struct{ items []models.NetworkAlert }

func (s *radioAlertStore) Read() ([]models.NetworkAlert, error) { return slices.Clone(s.items), nil }
func (s *radioAlertStore) Write(items []models.NetworkAlert) error {
	s.items = slices.Clone(items)
	return nil
}

// The ids of the alerts the Radio stack publishes and the ids the core has words for are one list.
func TestRadioStack_TheCoreHasWordsForEveryAlertTheStackPublishes(t *testing.T) {
	core, stk := radiowatch.AlertIDs(), radio.AlertIDs()
	slices.Sort(core)
	slices.Sort(stk)
	if !slices.Equal(core, stk) {
		t.Fatalf("the core words %v, the stack publishes %v", core, stk)
	}
}

// A Radio watch alert reaches the bell and no connector: the log holds it marked as shown in the app, and the connector send is never called.
func TestRadioStack_AWatchAlertGoesToTheBellOnlyAndNothingLooksUndelivered(t *testing.T) {
	store := &radioAlertStore{}
	log := netaudit.NewAlertLog(store, time.Now, nil)
	var s AppServices
	sink := stackalerts.Sink{Words: radiowatch.Words, Notify: s.bellNotifier(log, models.NetworkAlertRadio)}
	err := sink.Publish(context.Background(), stacksdk.Event{Type: stacksdk.EventAlert, Data: []byte(`{"id":"radio_watch_ending","hours":30}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 1 {
		t.Fatalf("%+v", store.items)
	}
	got := store.items[0]
	if got.Kind != models.NetworkAlertRadio || got.Delivery != models.NetworkAlertInApp || got.Message == "" {
		t.Errorf("%+v", got)
	}
}
