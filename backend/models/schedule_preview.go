package models

import "time"

// SchedulePreview is the answer to "what would this schedule do?": whether the
// trigger is valid, the zone its times are read in, and its next run times.
// It is computed from the scheduler's own parser and never stored.
type SchedulePreview struct {
	Valid    bool        `json:"valid"`
	Error    string      `json:"error,omitempty"`
	Timezone string      `json:"timezone"`
	NextRuns []time.Time `json:"next_runs"`
}
