package tools

import (
	"llm-proxy/internal/nodeherder"
	"testing"
	"time"
)

type MockClock struct {
	Now time.Time
}

func (m *MockClock) NowUtc() time.Time {
	return m.Now.UTC()
}

func TestIntentToMetricsArgs_Timezone(t *testing.T) {
	// Fixed "Now": 2025-06-15 16:00:00 UTC
	// In NY (EDT): 2025-06-15 12:00:00 (Noon)
	// Start of "today" in NY: 2025-06-15 00:00:00 EDT -> 2025-06-15 04:00:00 UTC
	// Start of "today" in UTC: 2025-06-15 00:00:00 UTC

	fixedNow := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	clock := &MockClock{Now: fixedNow}

	exposeIndex := map[ExposeKey]nodeherder.LLMExpose{
		{device: "test device", expose: "temp"}: {Name: "temp", Type: "numeric"},
	}

	tests := []struct {
		name     string
		intent   Intent
		timezone string
		wantFrom int64 // expected start time in ms
	}{
		{
			name: "Today in UTC",
			intent: Intent{
				Intent:     "latest_value",
				TargetName: "test device",
				Metrics:    []string{"temp"},
				TimeScope:  "today",
			},
			timezone: "", // Defaults to UTC
			// 2025-06-15 00:00:00 UTC
			wantFrom: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC).UnixMilli(),
		},
		{
			name: "Today in NY",
			intent: Intent{
				Intent:     "latest_value",
				TargetName: "test device",
				Metrics:    []string{"temp"},
				TimeScope:  "today",
			},
			timezone: "America/New_York",
			// 2025-06-15 00:00:00 EDT -> 04:00:00 UTC
			wantFrom: time.Date(2025, 6, 15, 4, 0, 0, 0, time.UTC).UnixMilli(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := IntentToMetricsArgs(tt.intent, clock, exposeIndex, tt.timezone)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(args) != 1 {
				t.Fatalf("expected 1 arg, got %d", len(args))
			}
			gotFrom := args[0].Time.From
			if gotFrom != tt.wantFrom {
				t.Errorf("Time.From = %d, want %d (diff %d ms)", gotFrom, tt.wantFrom, gotFrom-tt.wantFrom)
				// Helper to print readable times
				t.Logf("Got: %s", time.UnixMilli(gotFrom).UTC())
				t.Logf("Want: %s", time.UnixMilli(tt.wantFrom).UTC())
			}
		})
	}
}
