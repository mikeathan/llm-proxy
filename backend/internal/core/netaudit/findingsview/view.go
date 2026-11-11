// Package findingsview is the one reading of an audit's latest scan that the Findings screen and the assistant's network
// tools share: the findings with what is new, accepted or marked fixed, and what the last scan no longer shows. Building it
// is a pure function of already-read data (Of), the same shape as netaudit.OverviewOf; Reader does the reads.
package findingsview

import (
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

// Finding is a finding with whether the latest scan found it for the first time, and whether the operator has accepted it
// (and until when).
type Finding struct {
	models.ScanFinding
	// Key identifies the finding for accepting it; the page never builds one itself.
	Key string `json:"key"`
	New bool   `json:"new"`
	// CheckTitle names the check in a few plain words, so a screen can put every place the same problem was found under
	// one heading; empty for an open port.
	CheckTitle string `json:"check_title,omitempty"`
	// Fix is the suggested remedy for a check finding, fixed text the operator applies by hand; empty for an open port.
	Fix string `json:"fix,omitempty"`
	// Changed is true when the version differs from the scan before; PreviousVersion is what it was (possibly empty).
	Changed         bool   `json:"changed"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Accepted        bool   `json:"accepted"`
	AcceptedNote    string `json:"accepted_note,omitempty"`
	AcceptedTill    string `json:"accepted_until,omitempty"`
	// Guide is the longer, fixed-text form of the fix for a check (steps, commands to copy, risk, undo); absent for an
	// open port. FixState, FixMarkedAt and FixNote say what the operator told us about it: that they applied the fix,
	// when and with what note, and whether a scan since still shows it.
	Guide       *netaudit.FixGuide `json:"guide,omitempty"`
	FixState    string             `json:"fix_state,omitempty"`
	FixMarkedAt string             `json:"fix_marked_at,omitempty"`
	FixNote     string             `json:"fix_note,omitempty"`
	// FirstSeen is when this program first saw the finding and OpenSince when it last appeared (they differ after it came
	// back); CameBack counts the times it returned after being resolved. All are absent for a finding with no history yet.
	FirstSeen string `json:"first_seen,omitempty"`
	OpenSince string `json:"open_since,omitempty"`
	CameBack  int    `json:"came_back,omitempty"`
	// VerifiedFixedAt is when a recheck found the problem gone after the scan that still shows it; the next audit confirms it
	// (and the finding leaves the list) or it comes back.
	VerifiedFixedAt string `json:"verified_fixed_at,omitempty"`
	// Recheckable is whether "Check again" can judge this finding on its own (see netaudit.CanRecheck).
	Recheckable bool `json:"recheckable,omitempty"`
}

// Resolved is a finding the latest scan no longer shows, with when the operator marked it fixed if they had.
type Resolved struct {
	models.ScanFinding
	FixedAt string `json:"fixed_at,omitempty"`
	// ResolvedAt and ResolvedBy (audit or recheck) say when and how it went away, from the finding's history.
	ResolvedAt string `json:"resolved_at,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
}

// Report is an automation's latest scan as a screen needs it.
type Report struct {
	Scanned   bool       `json:"scanned"`
	ScannedAt string     `json:"scanned_at,omitempty"`
	Engine    string     `json:"engine,omitempty"`
	Findings  []Finding  `json:"findings"`
	Resolved  []Resolved `json:"resolved"`
	// Unscanned are the devices the engine ran out of time on; DevicesUp is how many addresses answered.
	Unscanned []string `json:"unscanned"`
	DevicesUp int      `json:"devices_up"`
	// FirstScan is true when this scan is the baseline: nothing came before it, so no open port in it is a change.
	FirstScan bool `json:"first_scan"`
}

// Input is everything Of needs, already read.
type Input struct {
	Baseline models.ScanBaseline
	Acks     map[string]models.ScanAck
	Fixes    map[string]models.FixRecord
	// DeepAt is when each device was last checked deeply. A deep finding is carried into every audit without being checked
	// again, so a fix mark on it is judged against that time and not against the nightly scan's.
	DeepAt map[string]time.Time
	// Life is each finding's history by key.
	Life map[string]models.FindingLife
	// Guide gives a finding its guide; netaudit.GuideFor when nil.
	Guide func(string) (netaudit.FixGuide, bool)
	Now   time.Time
}

// Of builds the report. A baseline that was never scanned gives Scanned false and empty (never nil) lists.
func Of(in Input) Report {
	b := in.Baseline
	rep := Report{Findings: []Finding{}, Resolved: []Resolved{}, Unscanned: []string{}}
	if b.ScannedAt.IsZero() {
		return rep
	}
	rep.Scanned, rep.ScannedAt, rep.Engine, rep.DevicesUp, rep.FirstScan = true, b.ScannedAt.UTC().Format(time.RFC3339), b.Engine, b.DevicesUp, b.FirstScan
	rep.Unscanned = append(rep.Unscanned, b.Unscanned...)
	v := newViewer(in)
	for _, f := range b.Findings {
		rep.Findings = append(rep.Findings, v.finding(f))
	}
	for _, r := range b.Resolved {
		rep.Resolved = append(rep.Resolved, v.resolved(r))
	}
	return rep
}

type viewer struct {
	in    Input
	isNew map[string]bool
	guide func(string) (netaudit.FixGuide, bool)
}

func newViewer(in Input) viewer {
	isNew := make(map[string]bool, len(in.Baseline.NewKeys))
	for _, k := range in.Baseline.NewKeys {
		isNew[k] = true
	}
	guide := in.Guide
	if guide == nil {
		guide = netaudit.GuideFor
	}
	return viewer{in: in, isNew: isNew, guide: guide}
}

// checkedAt is the time of the scan that last looked for this finding: the audit's, or for a deep check's finding the deep
// check's own.
func (v viewer) checkedAt(f models.ScanFinding) time.Time {
	if at, ok := v.in.DeepAt[f.Device]; ok && netaudit.IsDeepCheck(f.Check) {
		return at
	}
	return v.in.Baseline.ScannedAt
}

func (v viewer) finding(f models.ScanFinding) Finding {
	key := f.Key()
	view := Finding{ScanFinding: f, Key: key, New: v.isNew[key], CheckTitle: netaudit.CheckTitle(f.Check), Fix: netaudit.RemediationFor(f.Check)}
	view.PreviousVersion, view.Changed = v.in.Baseline.ChangedFrom[key]
	view.Recheckable = netaudit.CanRecheck(f)
	if g, ok := v.guide(f.Check); ok {
		view.Guide = &g
		if rec, marked := v.in.Fixes[key]; marked {
			view.FixState, view.FixNote = string(netaudit.FixStateFor(rec, v.checkedAt(f))), rec.Note
			view.FixMarkedAt = rec.AppliedAt.UTC().Format(time.RFC3339)
		}
	}
	if l, ok := v.in.Life[key]; ok {
		view.FirstSeen, view.OpenSince, view.CameBack = l.FirstSeen.UTC().Format(time.RFC3339), l.OpenSince.UTC().Format(time.RFC3339), l.Returned
		if !l.Open() && l.ResolvedBy == models.LifeByRecheck && l.ResolvedAt.After(v.in.Baseline.ScannedAt) {
			view.VerifiedFixedAt = l.ResolvedAt.UTC().Format(time.RFC3339)
		}
	}
	if a, ok := v.in.Acks[key]; ok && netaudit.StateFor(a, f, v.now()) == netaudit.AckActive {
		view.Accepted, view.AcceptedNote, view.AcceptedTill = true, a.Note, a.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return view
}

func (v viewer) now() time.Time {
	if v.in.Now.IsZero() {
		return time.Now()
	}
	return v.in.Now
}

func (v viewer) resolved(r models.ScanFinding) Resolved {
	view := Resolved{ScanFinding: r}
	if at, ok := v.in.Baseline.FixedAt[r.Key()]; ok {
		view.FixedAt = at.UTC().Format(time.RFC3339)
	}
	if l, ok := v.in.Life[r.Key()]; ok && !l.Open() {
		view.ResolvedAt, view.ResolvedBy = l.ResolvedAt.UTC().Format(time.RFC3339), l.ResolvedBy
	}
	return view
}
