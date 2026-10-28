package models

import "time"

// FixRecord is the operator's statement that they applied the fix for a finding: "I've done this". It names the finding by
// Key. It is a note for the operator and the report, nothing more: it never hides a finding, changes a comparison between
// scans or silences a notification. Whether the fix worked is decided by the next scan.
type FixRecord struct {
	Key       string    `json:"key"`
	AppliedAt time.Time `json:"applied_at"`
	Note      string    `json:"note,omitempty"`
}
