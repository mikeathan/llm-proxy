package automation

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestNew_CreatesCorrectTrigger(t *testing.T) {
	tests := []struct {
		name      string
		config    models.TriggerConfig
		wantType  models.TriggerType
		wantError bool
	}{
		{
			name:      "cron trigger",
			config:    models.TriggerConfig{Type: "cron", Value: "*/5 * * * *"},
			wantType:  "cron",
			wantError: false,
		},
		{
			name:      "interval trigger",
			config:    models.TriggerConfig{Type: "interval", Value: "15m"},
			wantType:  "interval",
			wantError: false,
		},
		{
			name:      "manual trigger",
			config:    models.TriggerConfig{Type: "manual", Value: ""},
			wantType:  "manual",
			wantError: false,
		},
		{
			name:      "unknown trigger",
			config:    models.TriggerConfig{Type: "unknown", Value: ""},
			wantType:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := New(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tr.Type() != tt.wantType {
				t.Errorf("Type() = %q, want %q", tr.Type(), tt.wantType)
			}
		})
	}
}

func TestCronTrigger_ShouldRun(t *testing.T) {
	tr, err := NewCronTrigger("*/5 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	// 18:00:00 is a cron mark
	baseTime := time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC)

	if !tr.ShouldRun(time.Time{}, baseTime) {
		t.Error("expected ShouldRun=true for zero lastRun")
	}

	// 10 minutes ago from 18:00:00 is 17:50:00 (also a mark)
	// Next(17:50:00) is 17:55:00, which is before 18:00:00.
	past := baseTime.Add(-10 * time.Minute)
	if !tr.ShouldRun(past, baseTime) {
		t.Error("expected ShouldRun=true for past lastRun")
	}

	// 1 minute ago from 18:00:00 is 17:59:00.
	// Next(17:59:00) is 18:00:00.
	// ShouldRun(17:59:00, 18:00:00) should be true because it hits the mark exactly.
	if !tr.ShouldRun(baseTime.Add(-1*time.Minute), baseTime) {
		t.Error("expected ShouldRun=true for recent lastRun crossing a boundary")
	}

	// 1 minute ago from 18:04:00 is 18:03:00.
	// Next(18:03:00) is 18:05:00.
	// ShouldRun(18:03:00, 18:04:00) should be false.
	recent := baseTime.Add(4 * time.Minute)
	lastRun := baseTime.Add(3 * time.Minute)
	if tr.ShouldRun(lastRun, recent) {
		t.Error("expected ShouldRun=false for recent lastRun not crossing a boundary")
	}
}

func TestCronTrigger_NextRun(t *testing.T) {
	tr, err := NewCronTrigger("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2026, 4, 2, 18, 30, 0, 0, time.UTC)
	next := tr.NextRun(baseTime)
	expected := time.Date(2026, 4, 2, 19, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("NextRun = %v, want %v", next, expected)
	}
}

func TestCronTrigger_Invalid(t *testing.T) {
	_, err := NewCronTrigger("invalid")
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestIntervalTrigger_ShouldRun(t *testing.T) {
	tr, err := NewIntervalTrigger("15m")
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC)

	if !tr.ShouldRun(time.Time{}, baseTime) {
		t.Error("expected ShouldRun=true for zero lastRun")
	}

	past := baseTime.Add(-20 * time.Minute)
	if !tr.ShouldRun(past, baseTime) {
		t.Error("expected ShouldRun=true for past lastRun")
	}

	recent := baseTime.Add(-5 * time.Minute)
	if tr.ShouldRun(recent, baseTime) {
		t.Error("expected ShouldRun=false for recent lastRun")
	}
}

func TestIntervalTrigger_NextRun(t *testing.T) {
	tr, err := NewIntervalTrigger("30m")
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC)
	next := tr.NextRun(baseTime)
	expected := baseTime.Add(30 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("NextRun = %v, want %v", next, expected)
	}
}

func TestIntervalTrigger_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid duration", value: "not-a-duration"},
		{name: "negative duration", value: "-15m"},
		{name: "zero duration", value: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewIntervalTrigger(tt.value)
			if err == nil {
				t.Error("expected error for invalid interval")
			}
		})
	}
}

func TestManualTrigger_ShouldRun(t *testing.T) {
	tr := &ManualTrigger{}
	now := time.Now()

	if tr.ShouldRun(now, now) {
		t.Error("expected ShouldRun=false for manual trigger")
	}
	if tr.ShouldRun(time.Time{}, now) {
		t.Error("expected ShouldRun=false for manual trigger even with zero time")
	}
}

func TestManualTrigger_NextRun(t *testing.T) {
	tr := &ManualTrigger{}
	next := tr.NextRun(time.Now())
	if !next.IsZero() {
		t.Error("expected zero time for manual trigger NextRun")
	}
}

func TestManualTrigger_New(t *testing.T) {
	tr, err := NewManualTrigger()
	if err != nil {
		t.Fatalf("NewManualTrigger() error = %v", err)
	}
	if tr.Type() != "manual" {
		t.Errorf("Type() = %q, want %q", tr.Type(), "manual")
	}
}
