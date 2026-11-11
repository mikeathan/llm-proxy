package models

import "time"

// FindingLife is what this program has seen of one finding over time, by Key: when it first appeared, when it was last
// shown, when it went away and whether the operator's recheck or the nightly audit said so, and how many times it came
// back. It is written only by the audit's report step and the recheck, never by a run's model, and it changes nothing about
// how scans are compared; the screens and the posture view read it.
type FindingLife struct {
	Key       string    `json:"key"`
	FirstSeen time.Time `json:"first_seen"`
	// OpenSince is when the finding last appeared (FirstSeen, or the audit that showed it again after it had gone).
	OpenSince time.Time `json:"open_since"`
	LastSeen  time.Time `json:"last_seen"`
	// ResolvedAt is zero while the finding is open. ResolvedBy is LifeByAudit or LifeByRecheck.
	ResolvedAt time.Time `json:"resolved_at,omitzero"`
	ResolvedBy string    `json:"resolved_by,omitempty"`
	// Returned counts the times it came back after being resolved.
	Returned int `json:"returned,omitempty"`
}

// Who ended a finding's life.
const (
	LifeByAudit   = "audit"
	LifeByRecheck = "recheck"
)

// Open reports whether the finding is still shown.
func (l FindingLife) Open() bool { return l.ResolvedAt.IsZero() }
