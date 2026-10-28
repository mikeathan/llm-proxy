package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type fakeFixes struct {
	records []models.FixRecord
	markErr error
	removed bool
	gotMark struct{ ws, automation, key, note string }
}

func (f *fakeFixes) List(string) ([]models.FixRecord, error) { return f.records, nil }
func (f *fakeFixes) Mark(ws, automation, key, note string) (models.FixRecord, error) {
	f.gotMark.ws, f.gotMark.automation, f.gotMark.key, f.gotMark.note = ws, automation, key, note
	return models.FixRecord{Key: key, Note: note}, f.markErr
}
func (f *fakeFixes) Unmark(string, string) (bool, error) { return f.removed, nil }

func TestNetauditHandlers_MarkFixed(t *testing.T) {
	fixes := &fakeFixes{}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithFixes(fixes)
	body := `{"workspace_id":"ws","automation":"nightly","key":"10.0.0.1|tcp|445|smb_v1","note":"turned it off"}`
	if rec := do(h.MarkFixed, http.MethodPost, "/admin/api/netaudit/fixes", body); rec.Code != http.StatusOK || fixes.gotMark.note != "turned it off" || fixes.gotMark.key == "" {
		t.Errorf("mark = %d %s (%+v)", rec.Code, rec.Body.String(), fixes.gotMark)
	}
	if rec := do(h.MarkFixed, http.MethodPost, "/", `{"workspace_id":"ws"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a request without the automation and key = %d", rec.Code)
	}
	for err, want := range map[error]int{netaudit.ErrBadFix: http.StatusBadRequest, netaudit.ErrNoSuchFinding: http.StatusNotFound, context.Canceled: http.StatusInternalServerError} {
		fixes.markErr = err
		if rec := do(h.MarkFixed, http.MethodPost, "/", body); rec.Code != want {
			t.Errorf("%v = %d, want %d", err, rec.Code, want)
		}
	}
}

func TestNetauditHandlers_UnmarkFixed(t *testing.T) {
	fixes := &fakeFixes{}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithFixes(fixes)
	if rec := do(h.UnmarkFixed, http.MethodDelete, "/?workspace_id=ws&key=k", ""); rec.Code != http.StatusNotFound {
		t.Errorf("taking back nothing = %d", rec.Code)
	}
	fixes.removed = true
	if rec := do(h.UnmarkFixed, http.MethodDelete, "/?workspace_id=ws&key=k", ""); rec.Code != http.StatusNoContent {
		t.Errorf("unmark = %d", rec.Code)
	}
	if rec := do(h.UnmarkFixed, http.MethodDelete, "/?workspace_id=ws", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a missing key = %d", rec.Code)
	}
}

func TestNetauditHandlers_FindingsCarryTheGuideAndWhatTheOperatorSaid(t *testing.T) {
	scannedAt := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	smb := models.ScanFinding{Device: "10.0.0.1", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	redis := models.ScanFinding{Device: "10.0.0.1", Port: 6379, Proto: "tcp", Check: netaudit.CheckRedisNoAuth, Severity: "critical"}
	ssh := models.ScanFinding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: netaudit.CheckOpenPort, Severity: "info"}
	gone := models.ScanFinding{Device: "10.0.0.2", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	baseline := models.ScanBaseline{
		ScannedAt: scannedAt, Engine: "nmap", Findings: []models.ScanFinding{smb, redis, ssh}, Resolved: []models.ScanFinding{gone},
		FixedAt: map[string]time.Time{gone.Key(): scannedAt.Add(-48 * time.Hour)},
	}
	fixes := &fakeFixes{records: []models.FixRecord{
		{Key: smb.Key(), AppliedAt: scannedAt.Add(-time.Hour), Note: "off"}, // a scan since: still there
		{Key: redis.Key(), AppliedAt: scannedAt.Add(time.Hour)},             // marked after the scan: waiting
	}}
	var got findingsResponse
	rec := getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, nil).WithFixes(fixes), "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]findingView{}
	for _, f := range got.Findings {
		byKey[f.Key] = f
	}
	if g := byKey[smb.Key()]; g.Guide == nil || len(g.Guide.Steps) == 0 || g.FixState != string(netaudit.FixStillPresent) || g.FixMarkedAt == "" || g.FixNote != "off" {
		t.Errorf("smb = %+v", g)
	}
	if g := byKey[redis.Key()]; g.Guide == nil || g.FixState != string(netaudit.FixMarked) {
		t.Errorf("redis = %+v", g)
	}
	if g := byKey[ssh.Key()]; g.Guide != nil || g.FixState != "" {
		t.Errorf("an open port has no guide and no state: %+v", g)
	}
	if len(got.Resolved) != 1 || got.Resolved[0].FixedAt == "" {
		t.Errorf("a resolved finding the operator had marked says when: %+v", got.Resolved)
	}
}

func TestNetauditHandlers_FindingsStillWorkWithoutFixRecords(t *testing.T) {
	smb := models.ScanFinding{Device: "10.0.0.1", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	baseline := models.ScanBaseline{ScannedAt: time.Now(), Engine: "nmap", Findings: []models.ScanFinding{smb}}
	var got findingsResponse
	rec := getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, nil), "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Findings[0].Guide == nil || got.Findings[0].FixState != "" {
		t.Errorf("the guide is there with no records at all: %s (%v)", rec.Body.String(), err)
	}
}
