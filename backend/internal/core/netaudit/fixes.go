package netaudit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/models"
)

// ErrBadFix means a request to mark a finding as fixed is not valid.
var ErrBadFix = errors.New("invalid fix record")

// FixStore keeps a workspace's fix records. *persistence.WorkspaceManager satisfies it.
type FixStore interface {
	ReadFixes(workspaceID string) ([]models.FixRecord, error)
	WriteFixes(workspaceID string, records []models.FixRecord) error
}

// Fixes is what the operator does after applying a fix by hand: say so, take that back, list the records. A record is a
// note, not a decision: it never hides a finding or changes how scans are compared.
type Fixes struct {
	store     FixStore
	baselines interface {
		ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	}
	now func() time.Time
}

func NewFixes(store FixStore, baselines interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
}) *Fixes {
	return &Fixes{store: store, baselines: baselines, now: time.Now}
}

// List returns the workspace's fix records.
func (f *Fixes) List(workspaceID string) ([]models.FixRecord, error) {
	return f.store.ReadFixes(workspaceID)
}

// Mark records that the operator applied the fix for a finding of the automation's latest scan. Only a finding with a fix
// guide can be marked: an open port is not a problem to fix. Marking again replaces the earlier record.
func (f *Fixes) Mark(workspaceID, automation, key, note string) (models.FixRecord, error) {
	baseline, err := f.baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		return models.FixRecord{}, fmt.Errorf("failed to read the latest scan: %w", err)
	}
	var found *Finding
	for i := range baseline.Findings {
		if baseline.Findings[i].Key() == key {
			found = &baseline.Findings[i]
			break
		}
	}
	if found == nil {
		return models.FixRecord{}, ErrNoSuchFinding
	}
	if _, ok := GuideFor(found.Check); !ok {
		return models.FixRecord{}, fmt.Errorf("%w: there is nothing to fix for this finding", ErrBadFix)
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxNoteLen {
		return models.FixRecord{}, fmt.Errorf("%w: the note is longer than %d characters", ErrBadFix, maxNoteLen)
	}
	rec := models.FixRecord{Key: key, AppliedAt: f.now(), Note: note}
	records, err := f.store.ReadFixes(workspaceID)
	if err != nil {
		return models.FixRecord{}, err
	}
	if err := f.store.WriteFixes(workspaceID, upsertFix(records, rec)); err != nil {
		return models.FixRecord{}, err
	}
	return rec, nil
}

// Unmark takes a record back; ok is false when there was none.
func (f *Fixes) Unmark(workspaceID, key string) (bool, error) {
	records, err := f.store.ReadFixes(workspaceID)
	if err != nil {
		return false, err
	}
	left, ok := removeFix(records, key)
	if !ok {
		return false, nil
	}
	return true, f.store.WriteFixes(workspaceID, left)
}

func upsertFix(records []models.FixRecord, rec models.FixRecord) []models.FixRecord {
	out, _ := removeFix(records, rec.Key)
	return append(out, rec)
}

func removeFix(records []models.FixRecord, key string) ([]models.FixRecord, bool) {
	out := make([]models.FixRecord, 0, len(records))
	for _, r := range records {
		if r.Key != key {
			out = append(out, r)
		}
	}
	return out, len(out) != len(records)
}

// FixState says what the latest scan shows about a finding the operator marked as fixed.
type FixState string

const (
	// FixMarked: no scan has run since the operator marked it; there is nothing to confirm yet.
	FixMarked FixState = "marked"
	// FixStillPresent: a scan finished after the operator marked it and the finding is still there.
	FixStillPresent FixState = "still_present"
)

// FixStateFor compares the record with the time of the saved scan that still shows the finding. A scan that was already
// running when the operator marked the finding is stamped with its end time, so it can be read as "after"; the wording
// the screen uses ("still showing") is true of that scan either way, and the next one settles it.
func FixStateFor(rec models.FixRecord, scannedAt time.Time) FixState {
	if scannedAt.After(rec.AppliedAt) {
		return FixStillPresent
	}
	return FixMarked
}

// ConfirmFixes drops the records of findings the latest scan no longer shows (their devices were scanned, so they are
// gone for real) and returns when each was marked, to be kept with the scan. Records of findings that are still there,
// or whose device was not looked at, stay.
func ConfirmFixes(records []models.FixRecord, resolved []Finding) (kept []models.FixRecord, confirmed map[string]time.Time) {
	gone := make(map[string]bool, len(resolved))
	for _, f := range resolved {
		gone[f.Key()] = true
	}
	for _, r := range records {
		if !gone[r.Key] {
			kept = append(kept, r)
			continue
		}
		if confirmed == nil {
			confirmed = map[string]time.Time{}
		}
		confirmed[r.Key] = r.AppliedAt
	}
	return kept, confirmed
}
