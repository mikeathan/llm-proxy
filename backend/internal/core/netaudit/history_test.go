package netaudit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type memHistory struct {
	h       models.NetworkHistory
	writes  int
	readErr error
}

func (m *memHistory) Read() (models.NetworkHistory, error) { return m.h, m.readErr }
func (m *memHistory) Write(h models.NetworkHistory) error {
	m.writes++
	m.h = h
	return nil
}

func historyAt(store *memHistory, now *time.Time) *History {
	h := NewHistory(store)
	h.now = func() time.Time { return *now }
	return h
}

func TestHistory_ALookIsOneEntryForTheDayWithTheMostDevicesUp(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 10, 31, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	for _, up := range []int{4, 7, 5} {
		if err := h.RecordLook(up, 9, 3); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.h.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(store.h.Days))
	}
	d := store.h.Days[0]
	if d.Date != "2026-10-31" || d.DevicesUp != 7 || d.Devices != 9 || d.Unnamed != 3 {
		t.Errorf("day = %+v", d)
	}
}

func TestHistory_AnUnchangedLookIsNotWrittenAgain(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 10, 31, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	for range 5 {
		_ = h.RecordLook(4, 9, 3)
	}
	if store.writes != 1 {
		t.Errorf("writes = %d, want 1: a presence check every ten minutes must not rewrite the file for nothing", store.writes)
	}
}

func TestHistory_AnAuditSetsTheFindingCountsAndKeepsTheLookCounts(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 10, 31, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	_ = h.RecordLook(6, 8, 2)
	findings := []models.ScanFinding{{Severity: SeverityCritical}, {Severity: SeverityHigh}, {Severity: SeverityLow}}
	if err := h.RecordAudit(findings); err != nil {
		t.Fatal(err)
	}
	d := store.h.Days[0]
	if d.Findings != 3 || d.Serious != 2 || !d.Audited || d.DevicesUp != 6 {
		t.Errorf("day = %+v", d)
	}
}

func TestHistory_IsBoundedAndKeptInDateOrder(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	for range maxHistoryDays + 50 {
		_ = h.RecordLook(1, 1, 0)
		now = now.AddDate(0, 0, 1)
	}
	if len(store.h.Days) != maxHistoryDays {
		t.Fatalf("days = %d, want %d", len(store.h.Days), maxHistoryDays)
	}
	for i := 1; i < len(store.h.Days); i++ {
		if store.h.Days[i-1].Date >= store.h.Days[i].Date {
			t.Fatalf("not in order at %d: %s then %s", i, store.h.Days[i-1].Date, store.h.Days[i].Date)
		}
	}
}

func TestHistory_SeriesReturnsOnlyTheLastDays(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	for range 40 {
		_ = h.RecordLook(1, 1, 0)
		now = now.AddDate(0, 0, 1)
	}
	got, err := h.Series(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 || got[len(got)-1].Date != "2026-11-09" {
		t.Errorf("series = %d days ending %s", len(got), got[len(got)-1].Date)
	}
}

func TestHistory_AReadFailureIsReportedAndNothingIsWritten(t *testing.T) {
	store, now := &memHistory{readErr: errors.New("disk")}, time.Date(2026, 10, 31, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	if err := h.RecordLook(1, 1, 0); err == nil || store.writes != 0 {
		t.Errorf("err = %v, writes = %d", err, store.writes)
	}
}

type auditSpy struct {
	got [][]models.ScanFinding
	err error
}

func (a *auditSpy) RecordAudit(f []models.ScanFinding) error { a.got = append(a.got, f); return a.err }

func TestRunReporter_ASavedAuditFeedsTheHistoryAndAFailureDoesNotChangeTheReport(t *testing.T) {
	spy, failures := &auditSpy{err: errors.New("disk full")}, 0
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime).WithHistory(spy, func(error) { failures++ })
	got := r.Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "assessment")
	if len(spy.got) != 1 || len(spy.got[0]) != 1 || failures != 1 {
		t.Errorf("recorded = %v, failures = %d", spy.got, failures)
	}
	if !strings.Contains(got, "assessment") || strings.Contains(got, "disk full") {
		t.Errorf("a history failure must not reach the report:\n%s", got)
	}
}

func TestRunReporter_AScanThatCouldNotBeSavedIsNotRecorded(t *testing.T) {
	spy := &auditSpy{}
	store := &memBaselines{data: map[string]models.ScanBaseline{}, writeErr: errors.New("read-only")}
	reporter(store, reportTime).WithHistory(spy, nil).Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "")
	if len(spy.got) != 0 {
		t.Error("only a saved audit counts")
	}
}

func TestHistory_TheDigestTimeSurvivesTheDailyWrites(t *testing.T) {
	store, now := &memHistory{}, time.Date(2026, 10, 31, 9, 0, 0, 0, time.Local)
	h := historyAt(store, &now)
	if last, err := h.LastDigest(); err != nil || !last.IsZero() {
		t.Fatalf("before the first summary: %v, %v", last, err)
	}
	if err := h.MarkDigest(now); err != nil {
		t.Fatal(err)
	}
	_ = h.RecordLook(3, 5, 1)
	_ = h.RecordAudit(nil)
	if last, _ := h.LastDigest(); !last.Equal(now) {
		t.Errorf("last digest = %v after the daily writes, want %v", last, now)
	}
}
