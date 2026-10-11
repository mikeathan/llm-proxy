package automation

import (
	"fmt"
	"time"

	"llm-proxy/models"
)

const (
	// DefaultSchedulePreview is how many upcoming runs a preview lists when the
	// caller does not ask for a number.
	DefaultSchedulePreview = 3
	// MaxSchedulePreview caps a preview so a request cannot make the server
	// iterate a schedule indefinitely.
	MaxSchedulePreview = 10

	secondsPerMinute = 60
	minutesPerHour   = 60
)

// PreviewSchedule reports whether a trigger is valid and when it would next
// run, using the same parser and trigger types the scheduler registers. Times
// are evaluated in the zone of now: the scheduler reads cron expressions in the
// server's local zone, so callers pass time.Now(). A manual trigger is valid
// and has no runs. Nothing is registered or stored.
func PreviewSchedule(cfg models.TriggerConfig, now time.Time, count int) models.SchedulePreview {
	preview := models.SchedulePreview{Timezone: zoneLabel(now), NextRuns: []time.Time{}}
	trigger, err := New(cfg)
	if err != nil {
		preview.Error = err.Error()
		return preview
	}
	preview.Valid = true
	if trigger.Type() == models.TriggerManual {
		return preview
	}
	count = clampPreviewCount(count)
	next := now
	for range count {
		next = trigger.NextRun(next)
		if next.IsZero() {
			break
		}
		preview.NextRuns = append(preview.NextRuns, next)
	}
	return preview
}

func clampPreviewCount(count int) int {
	switch {
	case count <= 0:
		return DefaultSchedulePreview
	case count > MaxSchedulePreview:
		return MaxSchedulePreview
	default:
		return count
	}
}

// zoneLabel names the zone of t the way an operator reads a clock: "UTC", or
// the abbreviation with its offset ("CEST (UTC+02:00)").
func zoneLabel(t time.Time) string {
	name, offset := t.Zone()
	if offset == 0 {
		return "UTC"
	}
	sign := '+'
	if offset < 0 {
		sign = '-'
		offset = -offset
	}
	minutes := offset / secondsPerMinute
	return fmt.Sprintf("%s (UTC%c%02d:%02d)", name, sign, minutes/minutesPerHour, minutes%minutesPerHour)
}
