package netaudit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type memFixes struct {
	records  []models.FixRecord
	readErr  error
	writeErr error
	writes   int
}

func (m *memFixes) ReadFixes(string) ([]models.FixRecord, error) { return m.records, m.readErr }
func (m *memFixes) WriteFixes(_ string, r []models.FixRecord) error {
	m.writes++
	if m.writeErr != nil {
		return m.writeErr
	}
	m.records = r
	return nil
}

var fixNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func smbFinding(device string) Finding {
	return Finding{Device: device, Port: 445, Proto: "tcp", Check: CheckSMBv1, Severity: SeverityHigh, Evidence: "SMBv1, an old and unsafe file-sharing version, is enabled"}
}

func newFixService(findings ...Finding) (*Fixes, *memFixes) {
	store := &memFixes{}
	svc := NewFixes(store, baselineReader{models.ScanBaseline{ScannedAt: fixNow.Add(-time.Hour), Findings: findings}})
	svc.now = func() time.Time { return fixNow }
	return svc, store
}

func TestFixes_MarkRecordsWhenTheOperatorSaidTheyDidIt(t *testing.T) {
	f := smbFinding("10.0.0.1")
	svc, store := newFixService(f, finding("10.0.0.2", 80, ""))
	rec, err := svc.Mark("ws", "nightly", f.Key(), "  turned SMBv1 off  ")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Key != f.Key() || !rec.AppliedAt.Equal(fixNow) || rec.Note != "turned SMBv1 off" || len(store.records) != 1 {
		t.Errorf("record = %+v, stored %+v", rec, store.records)
	}
}

func TestFixes_MarkingAgainReplacesTheRecordNotAddsOne(t *testing.T) {
	f := smbFinding("10.0.0.1")
	svc, store := newFixService(f)
	svc.Mark("ws", "nightly", f.Key(), "first")
	svc.now = func() time.Time { return fixNow.Add(time.Hour) }
	rec, _ := svc.Mark("ws", "nightly", f.Key(), "second")
	if len(store.records) != 1 || store.records[0].Note != "second" || !rec.AppliedAt.Equal(fixNow.Add(time.Hour)) {
		t.Errorf("stored %+v", store.records)
	}
}

func TestFixes_MarkRefusesWhatCannotBeFixedOrIsNotThere(t *testing.T) {
	port := finding("10.0.0.2", 80, "")
	f := smbFinding("10.0.0.1")
	svc, store := newFixService(f, port)
	if _, err := svc.Mark("ws", "nightly", "10.0.0.9|tcp|1|smb_v1", ""); !errors.Is(err, ErrNoSuchFinding) {
		t.Errorf("a finding the scan did not show: %v", err)
	}
	if _, err := svc.Mark("ws", "nightly", port.Key(), ""); !errors.Is(err, ErrBadFix) {
		t.Errorf("an open port has nothing to fix: %v", err)
	}
	if _, err := svc.Mark("ws", "nightly", f.Key(), strings.Repeat("x", 201)); !errors.Is(err, ErrBadFix) {
		t.Errorf("a long note: %v", err)
	}
	if len(store.records) != 0 {
		t.Errorf("nothing may be stored after a refusal: %+v", store.records)
	}
}

func TestFixes_UnmarkTakesTheRecordBack(t *testing.T) {
	f := smbFinding("10.0.0.1")
	svc, _ := newFixService(f)
	svc.Mark("ws", "nightly", f.Key(), "")
	if got, _ := svc.List("ws"); len(got) != 1 {
		t.Fatalf("list = %+v", got)
	}
	if ok, err := svc.Unmark("ws", f.Key()); !ok || err != nil {
		t.Errorf("unmark = %v, %v", ok, err)
	}
	if ok, _ := svc.Unmark("ws", f.Key()); ok {
		t.Error("unmarking twice reports nothing to take back")
	}
}

func TestFixStateFor_ASavedScanAfterTheRecordMeansItIsStillThere(t *testing.T) {
	rec := models.FixRecord{Key: "k", AppliedAt: fixNow}
	for name, tc := range map[string]struct {
		scannedAt time.Time
		want      FixState
	}{
		"scan before marking":    {fixNow.Add(-time.Minute), FixMarked},
		"scan at the same time":  {fixNow, FixMarked},
		"scan after the marking": {fixNow.Add(time.Minute), FixStillPresent},
	} {
		if got := FixStateFor(rec, tc.scannedAt); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
