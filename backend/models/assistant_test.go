package models

import "testing"

func TestSessionSource(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"wb_telegram_8699725510_20260709T151331Z", "webhook-telegram"},
		{"wb_slack_123_20260709T151331Z", "webhook-slack"},
		{"wb_discord_1_20260709T151331Z", "webhook-discord"},
		{"conv_abc123", "manual"},
		{"wb_", "manual"},
		{"", "manual"},
	}
	for _, c := range cases {
		if got := SessionSource(c.id); got != c.want {
			t.Errorf("SessionSource(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}
