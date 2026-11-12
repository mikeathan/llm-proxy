package netaudit

import (
	"time"

	"llm-proxy/models"
)

// The scan log is bounded so it cannot grow without end: a row older than ScanLogKeep goes, and past ScanLogMaxRows the
// oldest rows go first. A day holds a handful of scans (one scheduled, the rest pressed), so the bounds are generous.
const (
	ScanLogKeep        = 14 * 24 * time.Hour
	ScanLogMaxRows     = 200
	ScanLogKeysPerScan = 12
)

// ScanLogStore keeps a workspace's scan log. *persistence.WorkspaceManager satisfies it. Changes go through UpdateScanLog,
// which reads, changes and writes under one lock.
type ScanLogStore interface {
	ReadScanLog(workspaceID string) ([]models.ScanEntry, error)
	UpdateScanLog(workspaceID string, change func([]models.ScanEntry) []models.ScanEntry) error
}

// EntryOf is the log row for a saved scan. It is a pure function of the baseline that was just written: counts for
// everything, and the keys of the first few findings that were new or went away.
func EntryOf(automation string, next models.ScanBaseline, unattended bool) models.ScanEntry {
	e := models.ScanEntry{
		Automation: automation, At: next.ScannedAt, Unattended: unattended, Engine: next.Engine, DevicesUp: next.DevicesUp,
		New: len(next.NewKeys), Resolved: len(next.Resolved), NewKeys: firstKeys(next.NewKeys),
	}
	for _, f := range next.Findings {
		if f.Check == CheckOpenPort {
			e.OpenPorts++
			continue
		}
		if e.BySeverity == nil {
			e.BySeverity = map[string]int{}
		}
		e.BySeverity[f.Severity]++
	}
	gone := make([]string, len(next.Resolved))
	for i, f := range next.Resolved {
		gone[i] = f.Key()
	}
	e.ResolvedKeys = firstKeys(gone)
	return e
}

func firstKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	return append([]string(nil), keys[:min(len(keys), ScanLogKeysPerScan)]...)
}

// AppendEntry adds a row and applies the bounds, keeping the rows in the order they were added.
func AppendEntry(rows []models.ScanEntry, e models.ScanEntry, now time.Time) []models.ScanEntry {
	out := make([]models.ScanEntry, 0, len(rows)+1)
	for _, r := range rows {
		if now.Sub(r.At) <= ScanLogKeep {
			out = append(out, r)
		}
	}
	out = append(out, e)
	if len(out) > ScanLogMaxRows {
		out = out[len(out)-ScanLogMaxRows:]
	}
	return out
}
