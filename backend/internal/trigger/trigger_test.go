package trigger

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestNew_CreatesCorrectTrigger(t *testing.T) {
	tests := []struct {
		name      string
		config    models.TriggerConfig
		wantType  string
		wantError bool
	}{
		{
			name:     "cron trigger",
			config:   models.TriggerConfig{Type: "cron", Value: "*/5 * * * *"},
			wantType:  "cron",
			wantError: false,
		},
		{
			name:     "interval trigger",
			config:   models.TriggerConfig{Type: "interval", Value: "15m"},
			wantType:  "interval",
			wantError: false,
		},
		{
			name:     "manual trigger",
			config:   models.TriggerConfig{Type: "manual", Value: ""},
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

	if !tr.ShouldRun(time.Time{}) {
		t.Error("expected ShouldRun=true for zero lastRun")
	}

	past := time.Now().Add(-10 * time.Minute)
	if !tr.ShouldRun(past) {
		t.Error("expected ShouldRun=true for past lastRun")
	}

	recent := time.Now().Add(-1 * time.Minute)
	if tr.ShouldRun(recent) {
		t.Error("expected ShouldRun=false for recent lastRun")
	}
}

func TestCronTrigger_NextRun(t *testing.T) {
	tr, err := NewCronTrigger("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	next := tr.NextRun()
	if next.Before(time.Now()) {
		t.Error("NextRun should be in the future")
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

	if !tr.ShouldRun(time.Time{}) {
		t.Error("expected ShouldRun=true for zero lastRun")
	}

	past := time.Now().Add(-20 * time.Minute)
	if !tr.ShouldRun(past) {
		t.Error("expected ShouldRun=true for past lastRun")
	}

	recent := time.Now().Add(-5 * time.Minute)
	if tr.ShouldRun(recent) {
		t.Error("expected ShouldRun=false for recent lastRun")
	}
}

func TestIntervalTrigger_NextRun(t *testing.T) {
	tr, err := NewIntervalTrigger("30m")
	if err != nil {
		t.Fatal(err)
	}

	next := tr.NextRun()
	expected := time.Now().Add(30 * time.Minute)
	if next.Before(expected.Add(-time.Second)) || next.After(expected.Add(time.Second)) {
		t.Errorf("NextRun not within expected range")
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

	if tr.ShouldRun(time.Now()) {
		t.Error("expected ShouldRun=false for manual trigger")
	}
	if tr.ShouldRun(time.Time{}) {
		t.Error("expected ShouldRun=false for manual trigger even with zero time")
	}
}

func TestManualTrigger_NextRun(t *testing.T) {
	tr := &ManualTrigger{}
	next := tr.NextRun()
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
