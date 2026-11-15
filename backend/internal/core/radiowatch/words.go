// Package radiowatch holds the core's words for the alerts the Radio stack publishes about its long listening watch (ADR-0056). The stack names
// an id and a number of hours; the sentence is built here from a fixed table.
package radiowatch

import (
	"fmt"

	"llm-proxy-stacks/sdk"
)

// maxHours is the longest watch there is (7 days), so a number outside 1 to 168 is not one the stack could have meant.
const maxHours = 168

// Words is the sentence for an alert, or false for an id it does not know or hours that make no sense.
func Words(a stacksdk.Alert) (string, bool) {
	if a.Hours < 1 || a.Hours > maxHours {
		return "", false
	}
	switch a.ID {
	case "radio_watch_ending":
		return fmt.Sprintf("Radio: listening ends in about %s. Open Radio, then Listening, to start it again before there is a gap.", span(a.Hours)), true
	case "radio_watch_stopped":
		return fmt.Sprintf("Radio: listening stopped before its time, with about %s left. Open Radio, then Listening, to resume it; it asks for your passphrase.", span(a.Hours)), true
	}
	return "", false
}

func span(hours int) string {
	if hours == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", hours)
}

// AlertIDs are the alert ids Words has a sentence for, so a test can hold the table to the stack's list.
func AlertIDs() []string { return []string{"radio_watch_ending", "radio_watch_stopped"} }
