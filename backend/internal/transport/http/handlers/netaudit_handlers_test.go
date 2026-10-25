package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

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
	return NewNetauditHandlers(netaudit.NewScope(store, noResolver{}, nil), stubReadiness{}), store
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

type stubReadiness struct{ reqs []models.Requirement }

func (s stubReadiness) Check(context.Context) []models.Requirement { return s.reqs }

func TestNetauditHandlers_GetReadiness(t *testing.T) {
	get := func(r NetauditReadiness) map[string]any {
		h := NewNetauditHandlers(netaudit.NewScope(&scopeStore{}, noResolver{}, nil), r)
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
	h := NewNetauditHandlers(failingScope{netaudit.NewScope(store, noResolver{}, nil)}, stubReadiness{})
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
