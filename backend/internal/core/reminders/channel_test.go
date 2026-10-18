package reminders

import (
	"slices"
	"testing"
)

func policyWith(skipped []string, enabled ...string) ChannelPolicy {
	return ChannelPolicy{
		Connectors: func() []string { return enabled },
		Skipped:    func() []string { return skipped },
	}
}

// Where a reminder is delivered, given what was stored on it.
func TestChannelPolicy_Resolve(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		p      ChannelPolicy
		want   []string
	}{
		{"a named connector is used as stored", "slack", policyWith(nil, "telegram", "slack"), []string{"slack"}},
		{"a named connector that was skipped is still used: the user asked for it", "slack", policyWith([]string{"slack"}, "telegram", "slack"), []string{"slack"}},
		{"in-app is a choice of its own", InAppChannel, policyWith(nil, "telegram"), nil},
		{"unset goes to every enabled connector", "", policyWith(nil, "telegram", "slack"), []string{"telegram", "slack"}},
		{"unset skips the ones ticked off", "", policyWith([]string{"slack"}, "telegram", "slack"), []string{"telegram"}},
		{"unset with everything ticked off is in-app only", "", policyWith([]string{"telegram"}, "telegram"), nil},
		{"unset with no connector enabled is in-app only", "", policyWith(nil), nil},
		{"a skipped connector that is gone changes nothing", "", policyWith([]string{"discord"}, "telegram"), []string{"telegram"}},
	}
	for _, tc := range cases {
		if got := tc.p.Resolve(tc.stored); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChannelLabel(t *testing.T) {
	if got := ChannelLabel(nil); got != InAppChannel {
		t.Errorf("none = %q, want %q", got, InAppChannel)
	}
	if got := ChannelLabel([]string{"telegram", "slack"}); got != "telegram, slack" {
		t.Errorf("got %q", got)
	}
}

// What is stored when a reminder is created.
func TestChannelPolicy_ForNew(t *testing.T) {
	p := policyWith(nil, "telegram", "slack")
	cases := []struct {
		name          string
		named, origin string
		want          string
	}{
		{"a named channel wins", "slack", "telegram", "slack"},
		{"asked in a chat app: reply there", "", "slack", "slack"},
		{"asked in the web chat: leave it to the default", "", "", ""},
		{"the app it came from is no longer configured: the default", "", "discord", ""},
	}
	for _, tc := range cases {
		if got := p.ForNew(tc.named, tc.origin); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestChannelPolicy_ValidateNamed(t *testing.T) {
	p := policyWith(nil, "telegram")
	for _, ok := range []string{"", InAppChannel, "telegram"} {
		if err := p.ValidateNamed(ok); err != nil {
			t.Errorf("%q should be accepted: %v", ok, err)
		}
	}
	if err := p.ValidateNamed("slack"); err == nil {
		t.Error("an unconfigured connector must be refused")
	}
}

// The web UI is told about a delivery only when it is one of the places the reminder went; a failure or a miss is
// always shown, because the web UI is the only place those are ever reported.
func TestReminder_AlertsWebUI(t *testing.T) {
	cases := []struct {
		name      string
		connector string
		event     string
		want      bool
	}{
		{"default channel, delivered", "", EventDue, true},
		{"default channel, late", "", EventLate, true},
		{"default channel, partly delivered", "", EventPartial, true},
		{"in-app only, delivered", InAppChannel, EventDue, true},
		{"one named connector, delivered", "telegram", EventDue, false},
		{"one named connector, late", "telegram", EventLate, false},
		{"one named connector, failed", "telegram", EventFailed, true},
		{"one named connector, missed", "telegram", EventMissed, true},
		{"no event yet", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Reminder{Connector: tc.connector, Event: tc.event}).AlertsWebUI(); got != tc.want {
				t.Errorf("AlertsWebUI() = %v, want %v", got, tc.want)
			}
		})
	}
}
