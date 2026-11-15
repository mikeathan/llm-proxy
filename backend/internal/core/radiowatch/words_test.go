package radiowatch

import (
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func TestWordsAreFixedAndNameTheHoursAndTheWayToResume(t *testing.T) {
	ending, ok := Words(stacksdk.Alert{ID: "radio_watch_ending", Hours: 33})
	if !ok || !strings.Contains(ending, "33 hours") || !strings.Contains(ending, "Listening") {
		t.Errorf("%q %v", ending, ok)
	}
	stopped, ok := Words(stacksdk.Alert{ID: "radio_watch_stopped", Hours: 1})
	if !ok || !strings.Contains(stopped, "1 hour ") || !strings.Contains(stopped, "passphrase") {
		t.Errorf("%q %v", stopped, ok)
	}
}

func TestWordsRefuseAnIdOrHoursTheStackCouldNotHaveMeant(t *testing.T) {
	for name, a := range map[string]stacksdk.Alert{
		"unknown id": {ID: "radio_everything_is_fine", Hours: 3},
		"no hours":   {ID: "radio_watch_ending"},
		"too many":   {ID: "radio_watch_ending", Hours: 169},
		"negative":   {ID: "radio_watch_stopped", Hours: -4},
		"agents' id": {ID: "agent_secrets_found", Hours: 3},
		"empty":      {},
	} {
		if s, ok := Words(a); ok {
			t.Errorf("%s was worded: %q", name, s)
		}
	}
}
