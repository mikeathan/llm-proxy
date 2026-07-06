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
