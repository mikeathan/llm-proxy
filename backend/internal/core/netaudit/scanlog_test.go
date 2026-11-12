package netaudit

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func lt(h int) time.Time {
	return time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC).Add(time.Duration(h) * time.Hour)
}

func TestEntryOf_CountsAndNamesWhatChanged(t *testing.T) {
	next := models.ScanBaseline{
		ScannedAt: lt(2), Engine: "nmap", DevicesUp: 15,
		Findings: []models.ScanFinding{
			{Device: "10.0.0.1", Port: 23, Proto: "tcp", Check: CheckOpenPort, Severity: "info"},
			{Device: "10.0.0.1", Port: 80, Proto: "tcp", Check: CheckOpenPort, Severity: "info"},
			{Device: "10.0.0.1", Port: 23, Proto: "tcp", Check: "telnet_open", Severity: "high"},
		},
		NewKeys:  []string{"10.0.0.1|tcp|23|telnet_open"},
		Resolved: []models.ScanFinding{{Device: "10.0.0.9", Port: 8080, Proto: "tcp", Check: CheckOpenPort}},
	}
	e := EntryOf("network-audit", next, true)
	if e.OpenPorts != 2 || e.BySeverity["high"] != 1 || e.DevicesUp != 15 || !e.Unattended || e.Automation != "network-audit" {
		t.Errorf("entry wrong: %+v", e)
	}
	if e.New != 1 || e.Resolved != 1 || len(e.NewKeys) != 1 || e.ResolvedKeys[0] != "10.0.0.9|tcp|8080|open_port" {
		t.Errorf("changes wrong: %+v", e)
	}
}

func TestEntryOf_NamesAtMostAFewKeysButCountsAll(t *testing.T) {
	var keys []string
	for i := range 50 {
		keys = append(keys, fmt.Sprintf("10.0.0.1|tcp|%d|open_port", i))
	}
	e := EntryOf("a", models.ScanBaseline{ScannedAt: lt(0), NewKeys: keys}, false)
	if e.New != 50 || len(e.NewKeys) != ScanLogKeysPerScan {
		t.Errorf("count %d, named %d", e.New, len(e.NewKeys))
	}
}

func TestAppendEntry_DropsOldRowsByAgeAndCount(t *testing.T) {
	old := models.ScanEntry{Automation: "a", At: lt(0).Add(-ScanLogKeep - time.Hour)}
	fresh := models.ScanEntry{Automation: "a", At: lt(0)}
	got := AppendEntry([]models.ScanEntry{old}, fresh, lt(1))
	if len(got) != 1 || !got[0].At.Equal(fresh.At) {
		t.Errorf("an old row must go: %+v", got)
	}
	var many []models.ScanEntry
	for i := range ScanLogMaxRows + 20 {
		many = append(many, models.ScanEntry{Automation: "a", At: lt(0).Add(time.Duration(i) * time.Second)})
	}
	got = AppendEntry(many, models.ScanEntry{Automation: "a", At: lt(0).Add(time.Hour)}, lt(1))
	if len(got) != ScanLogMaxRows || !got[len(got)-1].At.Equal(lt(0).Add(time.Hour)) {
		t.Errorf("want %d rows ending with the new one, got %d", ScanLogMaxRows, len(got))
	}
}

type memScanLog struct {
	rows     []models.ScanEntry
	writeErr error
}

func (m *memScanLog) ReadScanLog(string) ([]models.ScanEntry, error) { return m.rows, nil }
func (m *memScanLog) UpdateScanLog(_ string, change func([]models.ScanEntry) []models.ScanEntry) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.rows = change(m.rows)
	return nil
}

func TestRunReporter_EachSavedScanAddsARowToTheScanLog(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	log := &memScanLog{}
	reporter(store, reportTime.Add(-time.Hour)).WithScanLog(log).Augment(models.WithUnattendedRun(scannedHosts(smbHost("10.0.0.1", false))), "ws", "nightly", "")
	reporter(store, reportTime).WithScanLog(log).Augment(scannedHosts(smbHost("10.0.0.1", true)), "ws", "nightly", "")
	if len(log.rows) != 2 {
		t.Fatalf("want 2 rows, got %+v", log.rows)
	}
	first, second := log.rows[0], log.rows[1]
	if !first.Unattended || second.Unattended || first.Automation != "nightly" || !second.At.Equal(reportTime) {
		t.Errorf("who started it, or when, is wrong: %+v %+v", first, second)
	}
	if second.Resolved != 1 || second.ResolvedKeys[0] != smbKey("10.0.0.1") {
		t.Errorf("the fix must show as resolved: %+v", second)
	}
}

func TestRunReporter_AFailingScanLogDoesNotStopTheAudit(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	log := &memScanLog{writeErr: errors.New("disk full")}
	got := reporter(store, reportTime).WithScanLog(log).Augment(scannedHosts(smbHost("10.0.0.1", false)), "ws", "nightly", "")
	if !strings.Contains(got, "scan log could not be updated") || len(store.data["ws/nightly"].Findings) == 0 {
		t.Errorf("the baseline must still be saved and the failure said: %q", got)
	}
}
