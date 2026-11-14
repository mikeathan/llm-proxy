package metrics

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

// The 2026 llama-server log prints prompt-processing speed on every progress
// line and on a "prompt eval time" summary; only the "eval time" line is the
// generation rate the dashboard means.
func TestTokenTracker_IgnoresPromptProcessingSpeed(t *testing.T) {
	tracker := NewTokenTracker()
	lines := []string{
		"0.12.029.775 I slot launch_slot_: id  0 | task 1 | processing task, is_child = 0",
		"0.17.850.118 I slot print_timing: id  0 | task 1 | prompt processing, n_tokens =   2048, progress = 0.34, t =   4.48 s / 457.32 tokens per second",
	}
	if _, err := tracker.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	if val, _ := tracker.LastTokensPerSecond(); val != 0 {
		t.Fatalf("prompt progress must not count as generation speed, got %v", val)
	}

	lines = []string{
		"0.32.042.873 I slot print_timing: id  0 | task 1 | prompt eval time =   18567.77 ms /  6088 tokens (    3.05 ms per token,   327.88 tokens per second)",
		"0.32.042.877 I slot print_timing: id  0 | task 1 |        eval time =    1445.20 ms /    45 tokens (   32.85 ms per token,    30.45 tokens per second)",
		"0.32.042.878 I slot print_timing: id  0 | task 1 |       total time =   20012.97 ms /  6133 tokens",
	}
	if _, err := tracker.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	if val, _ := tracker.LastTokensPerSecond(); val != 30.45 {
		t.Fatalf("expected generation speed 30.45, got %v", val)
	}
}
