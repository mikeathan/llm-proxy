package netaudit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func lf(check string, port int) Finding {
	return Finding{Device: "10.0.0.5", Port: port, Proto: "tcp", Check: check, Severity: "high"}
}

func lifeOf(t *testing.T, recs []models.FindingLife, f Finding) models.FindingLife {
	t.Helper()
	for _, r := range recs {
		if r.Key == f.Key() {
			return r
		}
	}
	t.Fatalf("no record for %s", f.Key())
	return models.FindingLife{}
}

func lifeDay(n int) time.Time { return time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC).AddDate(0, 0, n) }

func TestAdvanceLife_ANewFindingIsFirstSeenNow(t *testing.T) {
	f := lf(CheckSMBv1, 445)
	got := AdvanceLife(nil, []Finding{f}, nil, lifeDay(0))
	r := lifeOf(t, got, f)
	if !r.FirstSeen.Equal(lifeDay(0)) || !r.OpenSince.Equal(lifeDay(0)) || !r.Open() || r.Returned != 0 {
		t.Errorf("new record wrong: %+v", r)
	}
}

func TestAdvanceLife_AFindingStillThereKeepsItsFirstSeenAndMovesLastSeen(t *testing.T) {
	f := lf(CheckSMBv1, 445)
	got := AdvanceLife(AdvanceLife(nil, []Finding{f}, nil, lifeDay(0)), []Finding{f}, nil, lifeDay(3))
	r := lifeOf(t, got, f)
	if !r.FirstSeen.Equal(lifeDay(0)) || !r.LastSeen.Equal(lifeDay(3)) {
		t.Errorf("times: %+v", r)
	}
}

func TestAdvanceLife_AResolvedFindingIsClosedByTheAudit(t *testing.T) {
	f := lf(CheckSMBv1, 445)
	got := AdvanceLife(AdvanceLife(nil, []Finding{f}, nil, lifeDay(0)), nil, []Finding{f}, lifeDay(1))
	r := lifeOf(t, got, f)
	if r.Open() || !r.ResolvedAt.Equal(lifeDay(1)) || r.ResolvedBy != models.LifeByAudit {
		t.Errorf("not closed: %+v", r)
	}
}

// A finding that goes away and comes back is the one the operator must hear about: it is counted, and its open time restarts
// while its first-seen time does not.
func TestAdvanceLife_AFindingThatComesBackIsCountedAndReopened(t *testing.T) {
	f := lf(CheckSMBv1, 445)
	recs := AdvanceLife(nil, []Finding{f}, nil, lifeDay(0))
	recs = AdvanceLife(recs, nil, []Finding{f}, lifeDay(1))
	recs = AdvanceLife(recs, []Finding{f}, nil, lifeDay(2))
	r := lifeOf(t, recs, f)
	if !r.Open() || r.Returned != 1 || !r.FirstSeen.Equal(lifeDay(0)) || !r.OpenSince.Equal(lifeDay(2)) {
		t.Errorf("came back: %+v", r)
	}
}

// A resolved finding the records never held (they were created later) is not invented.
func TestAdvanceLife_AResolvedFindingWithNoRecordMakesNone(t *testing.T) {
	if got := AdvanceLife(nil, nil, []Finding{lf(CheckSMBv1, 445)}, lifeDay(1)); len(got) != 0 {
		t.Errorf("invented a record: %+v", got)
	}
}

// The records are bounded: an old resolved record is dropped, an open one never is.
func TestAdvanceLife_OldResolvedRecordsAreDroppedAndOpenOnesKept(t *testing.T) {
	gone, open := lf(CheckSMBv1, 445), lf(CheckVNCNoAuth, 23)
	recs := AdvanceLife(nil, []Finding{gone, open}, nil, lifeDay(0))
	recs = AdvanceLife(recs, []Finding{open}, []Finding{gone}, lifeDay(1))
	recs = AdvanceLife(recs, []Finding{open}, nil, lifeDay(1).Add(lifeKeepResolved+time.Hour))
	for _, r := range recs {
		if r.Key == gone.Key() {
			t.Errorf("an old resolved record was kept")
		}
	}
	lifeOf(t, recs, open)
}

func TestAdvanceLife_TheCapDropsTheOldestResolvedFirst(t *testing.T) {
	var cur []Finding
	for i := 0; i < lifeMaxRecords+5; i++ {
		cur = append(cur, lf(CheckSMBv1, 1000+i))
	}
	recs := AdvanceLife(nil, cur, nil, lifeDay(0))
	if len(recs) != lifeMaxRecords+5 {
		t.Fatalf("open records are never dropped, got %d", len(recs))
	}
	recs = AdvanceLife(recs, nil, cur, lifeDay(1))
	if len(recs) != lifeMaxRecords {
		t.Errorf("cap not applied: %d", len(recs))
	}
}

type memLife struct {
	records  []models.FindingLife
	readErr  error
	writeErr error
}

func (m *memLife) ReadLife(string) ([]models.FindingLife, error) { return m.records, m.readErr }
func (m *memLife) UpdateLife(_ string, change func([]models.FindingLife) []models.FindingLife) error {
	if m.readErr != nil {
		return m.readErr
	}
	next := change(m.records)
	if m.writeErr != nil {
		return m.writeErr
	}
	m.records = next
	return nil
}

func TestRunReporter_TheFindingHistoryFollowsEachSavedScan(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	life := &memLife{}
	run := func(at time.Time, up bool) {
		reporter(store, at).WithLife(life).Augment(scannedHosts(smbHost("10.0.0.1", up)), "ws", "nightly", "")
	}
	run(reportTime.Add(-48*time.Hour), false) // SMBv1 shown
	run(reportTime.Add(-24*time.Hour), true)  // gone
	run(reportTime, false)                    // back
	r := lifeOf(t, life.records, smbFinding("10.0.0.1"))
	if !r.Open() || r.Returned != 1 || !r.FirstSeen.Equal(reportTime.Add(-48*time.Hour)) {
		t.Errorf("history after gone-and-back: %+v", r)
	}
}

// A history that cannot be kept is said in the report and never stops the audit or the baseline.
func TestRunReporter_AFailingHistoryStoreDoesNotStopTheAudit(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	life := &memLife{writeErr: errors.New("disk full")}
	got := reporter(store, reportTime).WithLife(life).Augment(scannedHosts(smbHost("10.0.0.1", false)), "ws", "nightly", "")
	if !strings.Contains(got, "finding history could not be updated") || len(store.data["ws/nightly"].Findings) == 0 {
		t.Errorf("the baseline must still be saved and the failure said: %q", got)
	}
}
