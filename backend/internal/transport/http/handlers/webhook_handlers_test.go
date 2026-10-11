package handlers

import (
	"strings"
	"testing"
)

func TestWebhookSessionKey_Format(t *testing.T) {
	key := webhookSessionKey("telegram", "8699725510")
	parts := strings.Split(key, "_")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d: %s", len(parts), key)
	}
	if parts[0] != "wb" || parts[1] != "telegram" || parts[2] != "8699725510" {
		t.Errorf("unexpected format: %s", key)
	}
	// ISO UTC timestamp suffix is 16 chars: 20060102T150405Z
	if len(parts[3]) != 16 {
		t.Errorf("expected 16-char timestamp suffix, got %d: %q", len(parts[3]), parts[3])
	}
}

func TestWebhookHandler_AgentTimezone(t *testing.T) {
	cases := []struct {
		name string
		fn   func() string
		want string
	}{
		{"no provider", nil, "UTC"},
		{"unset", func() string { return "" }, "UTC"},
		{"valid owner zone", func() string { return "Europe/London" }, "Europe/London"},
		{"invalid owner zone falls back", func() string { return "Mars/Base" }, "UTC"},
	}
	for _, tc := range cases {
		h := &WebhookHandler{OwnerTimezone: tc.fn}
		if got := h.agentTimezone(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
