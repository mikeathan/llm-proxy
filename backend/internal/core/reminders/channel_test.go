package reminders

import (
	"slices"
	"testing"
)

func policyWith(def string, configured ...string) ChannelPolicy {
	return ChannelPolicy{
		Default:    func() string { return def },
		Configured: func(c string) bool { return slices.Contains(configured, c) },
	}
}

// Where a reminder is delivered, given what was stored on it.
func TestChannelPolicy_Resolve(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		p      ChannelPolicy
		want   string
	}{
		{"a named connector is used as stored", "slack", policyWith("telegram", "telegram", "slack"), "slack"},
		{"in-app is a choice of its own", InAppChannel, policyWith("telegram", "telegram"), InAppChannel},
		{"unset follows the default setting", "", policyWith("telegram", "telegram", "slack"), "telegram"},
		{"unset with no default is in-app only", "", policyWith("", "telegram"), InAppChannel},
		{"a single connector is not picked for you", "", policyWith("", "telegram"), InAppChannel},
		{"a default that is no longer configured is in-app only", "", policyWith("slack", "telegram"), InAppChannel},
	}
	for _, tc := range cases {
		if got := tc.p.Resolve(tc.stored); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// What is stored when a reminder is created.
func TestChannelPolicy_ForNew(t *testing.T) {
	p := policyWith("telegram", "telegram", "slack")
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
	p := policyWith("", "telegram")
	for _, ok := range []string{"", InAppChannel, "telegram"} {
		if err := p.ValidateNamed(ok); err != nil {
			t.Errorf("%q should be accepted: %v", ok, err)
		}
	}
	if err := p.ValidateNamed("slack"); err == nil {
		t.Error("an unconfigured connector must be refused")
	}
}
