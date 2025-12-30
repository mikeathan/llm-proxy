package system_metrics

import (
	"strings"
	"testing"
	"time"
)

func TestTokenTracker_ParsesTokensPerSecond(t *testing.T) {
	tracker := NewTokenTracker()

	input := strings.Join([]string{
		"starting up",
		"processed 12.5 tokens/s (avg)",
		"done",
	}, "\n") + "\n"

	if _, err := tracker.Write([]byte(input)); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	val, ts := tracker.LastTokensPerSecond()
	if val != 12.5 {
		t.Fatalf("expected 12.5, got %v", val)
	}
	if ts.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
	if time.Since(ts) < 0 {
		t.Fatalf("unexpected timestamp in the future")
	}
}

func TestTokenTracker_IgnoresInvalidLines(t *testing.T) {
	tracker := NewTokenTracker()

	input := strings.Join([]string{
		"nonsense",
		"tokens per second: nope",
		"12 tok/s", // valid
	}, "\n") + "\n"

	if _, err := tracker.Write([]byte(input)); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	val, _ := tracker.LastTokensPerSecond()
	if val != 12 {
		t.Fatalf("expected 12, got %v", val)
	}
}
