package automation

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestPreviewSchedule(t *testing.T) {
	now := time.Date(2026, 10, 12, 6, 30, 0, 0, time.UTC) // a Monday
	tests := []struct {
		name      string
		trigger   models.TriggerConfig
		count     int
		wantValid bool
		wantRuns  []time.Time
	}{
		{
			name:      "weekday morning lists the next three weekdays",
			trigger:   models.TriggerConfig{Type: models.TriggerCron, Value: "0 7 * * 1-5"},
			count:     3,
			wantValid: true,
			wantRuns: []time.Time{
				time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC),
				time.Date(2026, 10, 13, 7, 0, 0, 0, time.UTC),
				time.Date(2026, 10, 14, 7, 0, 0, 0, time.UTC),
			},
		},
		{
			name:      "descriptor",
			trigger:   models.TriggerConfig{Type: models.TriggerCron, Value: "@daily"},
			count:     1,
			wantValid: true,
			wantRuns:  []time.Time{time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:      "interval steps from now",
			trigger:   models.TriggerConfig{Type: models.TriggerInterval, Value: "1h"},
			count:     2,
			wantValid: true,
			wantRuns: []time.Time{
				time.Date(2026, 10, 12, 7, 30, 0, 0, time.UTC),
				time.Date(2026, 10, 12, 8, 30, 0, 0, time.UTC),
			},
		},
		{name: "manual is valid with no runs", trigger: models.TriggerConfig{Type: models.TriggerManual}, count: 3, wantValid: true},
		{name: "six fields are rejected", trigger: models.TriggerConfig{Type: models.TriggerCron, Value: "*/10 * * * * *"}, count: 3},
		{name: "garbage is rejected", trigger: models.TriggerConfig{Type: models.TriggerCron, Value: "every day"}, count: 3},
		{name: "empty cron is rejected", trigger: models.TriggerConfig{Type: models.TriggerCron}, count: 3},
		{name: "unknown type is rejected", trigger: models.TriggerConfig{Type: "file", Value: "x"}, count: 3},
		{name: "negative interval is rejected", trigger: models.TriggerConfig{Type: models.TriggerInterval, Value: "-5m"}, count: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PreviewSchedule(tt.trigger, now, tt.count)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v (error %q), want %v", got.Valid, got.Error, tt.wantValid)
			}
			if !tt.wantValid && got.Error == "" {
				t.Error("an invalid schedule must say why")
			}
			if tt.wantValid && got.Error != "" {
				t.Errorf("valid schedule carries an error: %q", got.Error)
			}
			if len(got.NextRuns) != len(tt.wantRuns) {
				t.Fatalf("NextRuns = %v, want %v", got.NextRuns, tt.wantRuns)
			}
			for i := range tt.wantRuns {
				if !got.NextRuns[i].Equal(tt.wantRuns[i]) {
					t.Errorf("NextRuns[%d] = %v, want %v", i, got.NextRuns[i], tt.wantRuns[i])
				}
			}
		})
	}
}

func TestPreviewSchedule_CountIsBounded(t *testing.T) {
	trigger := models.TriggerConfig{Type: models.TriggerCron, Value: "* * * * *"}
	now := time.Date(2026, 10, 12, 6, 30, 0, 0, time.UTC)
	if got := PreviewSchedule(trigger, now, 1000); len(got.NextRuns) != MaxSchedulePreview {
		t.Errorf("len = %d, want the cap %d", len(got.NextRuns), MaxSchedulePreview)
	}
	if got := PreviewSchedule(trigger, now, 0); len(got.NextRuns) != DefaultSchedulePreview {
		t.Errorf("len = %d, want the default %d", len(got.NextRuns), DefaultSchedulePreview)
	}
}

func TestPreviewSchedule_NamesTheZoneOfNow(t *testing.T) {
	manual := models.TriggerConfig{Type: models.TriggerManual}
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{"utc", time.Date(2026, 10, 12, 6, 30, 0, 0, time.UTC), "UTC"},
		{"east", time.Date(2026, 10, 12, 6, 30, 0, 0, time.FixedZone("CEST", 2*3600)), "CEST (UTC+02:00)"},
		{"west half hour", time.Date(2026, 10, 12, 6, 30, 0, 0, time.FixedZone("NST", -(3*3600+1800))), "NST (UTC-03:30)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PreviewSchedule(manual, tt.now, 1); got.Timezone != tt.want {
				t.Errorf("Timezone = %q, want %q", got.Timezone, tt.want)
			}
		})
	}
}

// Next runs are evaluated in the zone of now, which is how the scheduler
// (running in the server's local zone) reads the expression.
func TestPreviewSchedule_EvaluatesInTheZoneOfNow(t *testing.T) {
	zone := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 10, 12, 6, 30, 0, 0, zone)
	got := PreviewSchedule(models.TriggerConfig{Type: models.TriggerCron, Value: "0 7 * * *"}, now, 1)
	want := time.Date(2026, 10, 12, 7, 0, 0, 0, zone)
	if len(got.NextRuns) != 1 || !got.NextRuns[0].Equal(want) {
		t.Errorf("NextRuns = %v, want %v", got.NextRuns, want)
	}
}
