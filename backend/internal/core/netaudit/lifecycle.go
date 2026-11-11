package netaudit

import (
	"sort"
	"time"

	"llm-proxy/models"
)

// The life records are bounded so they cannot grow without end: a resolved record older than lifeKeepResolved goes, and past
// lifeMaxRecords the oldest resolved ones go first. An open record is never dropped.
const (
	lifeKeepResolved = 180 * 24 * time.Hour
	lifeMaxRecords   = 2000
)

// LifeStore keeps a workspace's finding life records. *persistence.WorkspaceManager satisfies it. Changes go through UpdateLife,
// which reads, changes and writes under one lock, because the nightly report step and the operator's recheck both write.
type LifeStore interface {
	ReadLife(workspaceID string) ([]models.FindingLife, error)
	UpdateLife(workspaceID string, change func([]models.FindingLife) []models.FindingLife) error
}

// AdvanceLife moves the records on by one scan: every finding the scan shows is open (first seen now if new, counted as
// having come back if it had been resolved), and every finding the scan no longer shows is closed by the audit. It is a pure
// function of its inputs; the report step reads and writes the store around it.
func AdvanceLife(records []models.FindingLife, current, resolved []Finding, at time.Time) []models.FindingLife {
	byKey := make(map[string]models.FindingLife, len(records)+len(current))
	for _, r := range records {
		byKey[r.Key] = r
	}
	for _, f := range current {
		byKey[f.Key()] = showAgain(byKey[f.Key()], f.Key(), at)
	}
	for _, f := range resolved {
		if r, ok := byKey[f.Key()]; ok && r.Open() {
			r.ResolvedAt, r.ResolvedBy = at, models.LifeByAudit
			byKey[f.Key()] = r
		}
	}
	return pruneLife(byKey, at)
}

// showAgain is a record after a scan that shows its finding.
func showAgain(r models.FindingLife, key string, at time.Time) models.FindingLife {
	switch {
	case r.Key == "":
		return models.FindingLife{Key: key, FirstSeen: at, OpenSince: at, LastSeen: at}
	case !r.Open():
		r.Returned++
		r.OpenSince, r.ResolvedAt, r.ResolvedBy = at, time.Time{}, ""
	}
	r.LastSeen = at
	return r
}

// pruneLife applies the bounds and returns the records in key order, so the stored file is stable.
func pruneLife(byKey map[string]models.FindingLife, at time.Time) []models.FindingLife {
	out := make([]models.FindingLife, 0, len(byKey))
	for _, r := range byKey {
		if r.Open() || at.Sub(r.ResolvedAt) <= lifeKeepResolved {
			out = append(out, r)
		}
	}
	if len(out) > lifeMaxRecords {
		out = capResolved(out)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// capResolved drops the oldest resolved records until the cap holds, or only open ones remain.
func capResolved(recs []models.FindingLife) []models.FindingLife {
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Open() != recs[j].Open() {
			return recs[i].Open()
		}
		return recs[i].ResolvedAt.After(recs[j].ResolvedAt)
	})
	keep := len(recs)
	for keep > lifeMaxRecords && !recs[keep-1].Open() {
		keep--
	}
	return recs[:keep]
}

// CloseLife marks one finding resolved by who (the recheck). A finding with no record yet gets one, first seen now, so the
// answer is not lost. A finding that is already resolved keeps its first answer.
func CloseLife(records []models.FindingLife, f Finding, by string, at time.Time) []models.FindingLife {
	byKey := make(map[string]models.FindingLife, len(records)+1)
	for _, r := range records {
		byKey[r.Key] = r
	}
	r, ok := byKey[f.Key()]
	if !ok {
		r = models.FindingLife{Key: f.Key(), FirstSeen: at, OpenSince: at, LastSeen: at}
	}
	if r.Open() {
		r.ResolvedAt, r.ResolvedBy = at, by
	}
	byKey[f.Key()] = r
	return pruneLife(byKey, at)
}
