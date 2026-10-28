package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type scopeStore struct{ cfg models.NetauditConfig }

func (s *scopeStore) Get() models.NetauditConfig { return s.cfg }
func (s *scopeStore) Update(fn func(*models.NetauditConfig) error) error {
	next := s.cfg
	if err := fn(&next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

type noResolver struct{}

func (noResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, context.Canceled
}

func newNetauditTestHandlers() (*NetauditHandlers, *scopeStore) {
	store := &scopeStore{}
	return NewNetauditHandlers(netaudit.NewScope(store, noResolver{}, nil), stubReadiness{}, nil, nil), store
}

func putScope(h *NetauditHandlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/admin/api/netaudit/scope", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.PutScope(rec, req)
	return rec
}

func TestNetauditHandlers_GetScopeStartsOff(t *testing.T) {
	h, _ := newNetauditTestHandlers()
	rec := httptest.NewRecorder()
	h.GetScope(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/scope", nil))
	var got struct {
		Ranges      []any `json:"ranges"`
		Suggestions []any `json:"suggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Ranges == nil || got.Suggestions == nil || len(got.Ranges) != 0 {
		t.Errorf("a fresh scope must list empty arrays, not null: %s (%v)", rec.Body.String(), err)
	}
}

func TestNetauditHandlers_PutScopeSavesAndRefuses(t *testing.T) {
	h, store := newNetauditTestHandlers()

	if rec := putScope(h, `{"entries":["192.168.1.0/24"]}`); rec.Code != http.StatusOK {
		t.Fatalf("a private range = %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.cfg.ScanRanges) != 1 {
		t.Errorf("stored = %v", store.cfg.ScanRanges)
	}

	for _, bad := range []string{`{"entries":["8.8.8.8"]}`, `{"entries":["not a target"]}`} {
		if rec := putScope(h, bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rec.Code)
		}
	}
	if len(store.cfg.ScanRanges) != 1 {
		t.Errorf("a refused save must leave the scope as it was: %v", store.cfg.ScanRanges)
	}
}

func TestNetauditHandlers_PutScopeLeavesOmittedFieldsAlone(t *testing.T) {
	h, store := newNetauditTestHandlers()
	store.cfg = models.NetauditConfig{ScanRanges: []string{"10.0.0.0/24"}}

	if rec := putScope(h, `{"allow_unconfined":true}`); rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !store.cfg.AllowUnconfined || len(store.cfg.ScanRanges) != 1 {
		t.Errorf("only the switch may change: %+v", store.cfg)
	}
	if rec := putScope(h, `{"entries":[]}`); rec.Code != http.StatusOK || len(store.cfg.ScanRanges) != 0 || !store.cfg.AllowUnconfined {
		t.Errorf("an empty list turns scanning off and keeps the switch: %+v (%d)", store.cfg, rec.Code)
	}
}

type stubReadiness struct{ reqs, notices []models.Requirement }

func (s stubReadiness) Check(context.Context) []models.Requirement { return s.reqs }
func (s stubReadiness) Notices() []models.Requirement                { return s.notices }

func TestNetauditHandlers_GetReadiness(t *testing.T) {
	get := func(r NetauditReadiness) map[string]any {
		h := NewNetauditHandlers(netaudit.NewScope(&scopeStore{}, noResolver{}, nil), r, nil, nil)
		rec := httptest.NewRecorder()
		h.GetReadiness(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/readiness", nil))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	ready := get(stubReadiness{})
	if ready["ready"] != true || ready["requirements"] == nil {
		t.Errorf("ready host = %v (requirements must be [], not null)", ready)
	}
	withNotice := get(stubReadiness{notices: []models.Requirement{{Code: models.RequirementVulnDataMissing, Message: "n"}}})
	if withNotice["ready"] != true || len(withNotice["notices"].([]any)) != 1 || ready["notices"] == nil {
		t.Errorf("a notice must not make the host not ready, and notices must be [] not null: %v / %v", withNotice, ready)
	}
	missing := get(stubReadiness{reqs: []models.Requirement{{Code: models.RequirementScopeEmpty, Blocking: true, Message: "m"}}})
	if missing["ready"] != false || len(missing["requirements"].([]any)) != 1 {
		t.Errorf("missing = %v", missing)
	}
}

type failingScope struct{ NetauditScope }

func (failingScope) Suggestions() ([]netaudit.Suggestion, error) {
	return nil, context.DeadlineExceeded
}

func TestNetauditHandlers_AFailureToListNetworksIsReportedNotHidden(t *testing.T) {
	store := &scopeStore{cfg: models.NetauditConfig{ScanRanges: []string{"10.0.0.0/24"}}}
	h := NewNetauditHandlers(failingScope{netaudit.NewScope(store, noResolver{}, nil)}, stubReadiness{}, nil, nil)
	rec := httptest.NewRecorder()
	h.GetScope(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/scope", nil))
	var got struct {
		Ranges           []any  `json:"ranges"`
		Suggestions      []any  `json:"suggestions"`
		SuggestionsError string `json:"suggestions_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Ranges) != 1 || got.Suggestions == nil || got.SuggestionsError == "" {
		t.Errorf("the saved scope must still be returned, with the reason the list is empty: %s", rec.Body.String())
	}
}

type fixedBaselines struct {
	b   models.ScanBaseline
	err error
}

func (f fixedBaselines) ReadBaseline(string, string) (models.ScanBaseline, error) { return f.b, f.err }

func getFindings(h *NetauditHandlers, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.GetFindings(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/findings"+query, nil))
	return rec
}

func TestNetauditHandlers_GetFindings(t *testing.T) {
	ssh := models.ScanFinding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: "open_port", Severity: "info"}
	telnet := models.ScanFinding{Device: "10.0.0.2", Port: 23, Proto: "tcp", Check: "open_port", Severity: "info"}
	gone := models.ScanFinding{Device: "10.0.0.3", Port: 445, Proto: "tcp", Check: "open_port", Severity: "info"}
	baseline := models.ScanBaseline{
		ScannedAt: time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC), Engine: "nmap",
		Findings: []models.ScanFinding{ssh, telnet}, NewKeys: []string{telnet.Key()}, Resolved: []models.ScanFinding{gone},
		ChangedFrom: map[string]string{ssh.Key(): "9.6p1"}, Unscanned: []string{"10.0.0.9"}, DevicesUp: 10,
	}
	h := NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, nil)

	var got findingsResponse
	rec := getFindings(h, "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if got.Findings[0].Key != ssh.Key() || got.Findings[1].Key != telnet.Key() {
		t.Errorf("each finding carries the key to accept it by: %+v", got.Findings)
	}
	if !got.Findings[0].Changed || got.Findings[0].PreviousVersion != "9.6p1" || got.Findings[1].Changed {
		t.Errorf("only the ssh finding changed, from 9.6p1: %+v", got.Findings)
	}
	if len(got.Unscanned) != 1 || got.Unscanned[0] != "10.0.0.9" || got.DevicesUp != 10 {
		t.Errorf("unscanned devices and devices up must be returned: %+v", got)
	}
	if !got.Scanned || got.Engine != "nmap" || len(got.Findings) != 2 || got.Findings[0].New || !got.Findings[1].New || len(got.Resolved) != 1 {
		t.Errorf("findings = %+v", got)
	}

	var none findingsResponse
	rec = getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{}, nil), "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &none); err != nil || none.Scanned || none.Findings == nil || none.Resolved == nil {
		t.Errorf("an automation that has not scanned: %s", rec.Body.String())
	}
	if rec := getFindings(h, "?workspace_id=ws"); rec.Code != http.StatusBadRequest {
		t.Errorf("a missing automation = %d", rec.Code)
	}
	if rec := getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{err: context.Canceled}, nil), "?workspace_id=ws&automation=a"); rec.Code != http.StatusInternalServerError {
		t.Errorf("a read failure = %d", rec.Code)
	}
}

type fakeAcks struct {
	acks      []models.ScanAck
	acceptErr error
	revoked   bool
	gotAccept ackRequest
}

func (f *fakeAcks) List(string) ([]models.ScanAck, error) { return f.acks, nil }
func (f *fakeAcks) Accept(ws, automation, key, note string, days int) (models.ScanAck, error) {
	f.gotAccept = ackRequest{ws, automation, key, note, days}
	return models.ScanAck{Key: key, Note: note}, f.acceptErr
}
func (f *fakeAcks) Revoke(string, string) (bool, error) { return f.revoked, nil }

func do(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestNetauditHandlers_AcceptFinding(t *testing.T) {
	acks := &fakeAcks{}
	h := NewNetauditHandlers(nil, nil, nil, acks)
	body := `{"workspace_id":"ws","automation":"nightly","key":"10.0.0.1|tcp|22|open_port","note":"jump host","days":30}`

	if rec := do(h.AcceptFinding, http.MethodPost, "/admin/api/netaudit/acks", body); rec.Code != http.StatusOK || acks.gotAccept.Days != 30 || acks.gotAccept.Note != "jump host" {
		t.Errorf("accept = %d %s (%+v)", rec.Code, rec.Body.String(), acks.gotAccept)
	}
	if rec := do(h.AcceptFinding, http.MethodPost, "/", `{"workspace_id":"ws"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a request without the automation and key = %d", rec.Code)
	}
	for err, want := range map[error]int{netaudit.ErrBadAck: http.StatusBadRequest, netaudit.ErrNoSuchFinding: http.StatusNotFound, context.Canceled: http.StatusInternalServerError} {
		acks.acceptErr = err
		if rec := do(h.AcceptFinding, http.MethodPost, "/", body); rec.Code != want {
			t.Errorf("%v = %d, want %d", err, rec.Code, want)
		}
	}
}

func TestNetauditHandlers_ListAndRevokeAcks(t *testing.T) {
	acks := &fakeAcks{}
	h := NewNetauditHandlers(nil, nil, nil, acks)
	rec := do(h.ListAcks, http.MethodGet, "/admin/api/netaudit/acks?workspace_id=ws", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("no acceptances is an empty array, not null: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(h.ListAcks, http.MethodGet, "/admin/api/netaudit/acks", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a missing workspace = %d", rec.Code)
	}
	if rec := do(h.RevokeAck, http.MethodDelete, "/?workspace_id=ws&key=k", ""); rec.Code != http.StatusNotFound {
		t.Errorf("revoking nothing = %d", rec.Code)
	}
	acks.revoked = true
	if rec := do(h.RevokeAck, http.MethodDelete, "/?workspace_id=ws&key=k", ""); rec.Code != http.StatusNoContent {
		t.Errorf("revoke = %d", rec.Code)
	}
	if rec := do(h.RevokeAck, http.MethodDelete, "/?workspace_id=ws", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a missing key = %d", rec.Code)
	}
}

func TestNetauditHandlers_FindingsShowWhatIsAccepted(t *testing.T) {
	ssh := models.ScanFinding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: "open_port", Severity: "info", Version: "OpenSSH 9"}
	telnet := models.ScanFinding{Device: "10.0.0.1", Port: 23, Proto: "tcp", Check: "open_port", Severity: "info"}
	baseline := models.ScanBaseline{ScannedAt: time.Now(), Engine: "nmap", Findings: []models.ScanFinding{ssh, telnet}}
	now := time.Now()
	acks := &fakeAcks{acks: []models.ScanAck{
		{Key: ssh.Key(), AcceptedVersion: "OpenSSH 9", Note: "jump host", ExpiresAt: now.Add(24 * time.Hour)},
		{Key: telnet.Key(), ExpiresAt: now.Add(-time.Hour)}, // expired: not accepted any more
	}}
	var got findingsResponse
	rec := getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, acks), "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Findings[0].Accepted || got.Findings[0].AcceptedNote != "jump host" || got.Findings[0].AcceptedTill == "" || got.Findings[1].Accepted {
		t.Errorf("findings = %+v", got.Findings)
	}
}

func TestNetauditHandlers_FindingsCarryTheSuggestedFixOfACheck(t *testing.T) {
	smb := models.ScanFinding{Device: "10.0.0.1", Port: 445, Proto: "tcp", Check: netaudit.CheckSMBv1, Severity: "high"}
	port := models.ScanFinding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: netaudit.CheckOpenPort, Severity: "info"}
	baseline := models.ScanBaseline{ScannedAt: time.Now(), Engine: "nmap", Findings: []models.ScanFinding{smb, port}}
	var got findingsResponse
	rec := getFindings(NewNetauditHandlers(nil, nil, fixedBaselines{b: baseline}, nil), "?workspace_id=ws&automation=nightly")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Findings[0].Fix != netaudit.RemediationFor(netaudit.CheckSMBv1) || got.Findings[0].Fix == "" || got.Findings[1].Fix != "" {
		t.Errorf("only the check finding has a fix: %+v", got.Findings)
	}
}

func TestNetauditHandlers_PutScopeSetsThePerDeviceTimeAndRefusesOtherChoices(t *testing.T) {
	h, store := newNetauditTestHandlers()

	rec := putScope(h, `{"device_timeout_minutes":5}`)
	var got struct {
		DeviceTimeoutMinutes int `json:"device_timeout_minutes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || got.DeviceTimeoutMinutes != 5 || store.cfg.DeviceTimeoutMinutes != 5 {
		t.Fatalf("code %d, body %s, stored %d", rec.Code, rec.Body.String(), store.cfg.DeviceTimeoutMinutes)
	}
	if rec := putScope(h, `{"device_timeout_minutes":45}`); rec.Code != http.StatusBadRequest || store.cfg.DeviceTimeoutMinutes != 5 {
		t.Errorf("a time that is not offered = %d and must change nothing (stored %d)", rec.Code, store.cfg.DeviceTimeoutMinutes)
	}
	if rec := putScope(h, `{"allow_unconfined":true}`); rec.Code != http.StatusOK || store.cfg.DeviceTimeoutMinutes != 5 {
		t.Errorf("a change to another field must leave the time alone (stored %d)", store.cfg.DeviceTimeoutMinutes)
	}
}

func TestNetauditHandlers_CheckTargetsListsWhatTheAuditCouldNotScan(t *testing.T) {
	h, store := newNetauditTestHandlers()
	store.cfg.ScanRanges = []string{"192.168.1.0/24"}

	req := httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/targets/check", strings.NewReader(`{"targets":["192.168.1.5","10.1.1.1"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CheckTargets(rec, req)

	var got struct {
		Problems []netaudit.TargetProblem `json:"problems"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if len(got.Problems) != 1 || got.Problems[0].Entry != "10.1.1.1" {
		t.Errorf("problems = %+v", got.Problems)
	}

	req = httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/targets/check", strings.NewReader(`{"targets":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.CheckTargets(rec, req)
	if !strings.Contains(rec.Body.String(), `"problems":[]`) {
		t.Errorf("no problems is an empty list, not null: %s", rec.Body.String())
	}
}
