package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type fakeExpectations struct {
	items     []models.Expectation
	gotSet    expectationRequest
	setErr    error
	removeOK  bool
	removeErr error
}

func (f *fakeExpectations) List(string) ([]models.Expectation, error) { return f.items, nil }
func (f *fakeExpectations) Set(_ context.Context, ws, device string, ports []int, note string) (models.Expectation, error) {
	f.gotSet = expectationRequest{WorkspaceID: ws, Device: device, Ports: ports, Note: note}
	return models.Expectation{Device: device, Ports: ports, Note: note}, f.setErr
}
func (f *fakeExpectations) Remove(string, string) (bool, error) { return f.removeOK, f.removeErr }

func TestNetauditHandlers_ListExpectations(t *testing.T) {
	h := NewNetauditHandlers(nil, nil, nil, nil).WithExpectations(&fakeExpectations{})
	rec := do(h.ListExpectations, http.MethodGet, "/admin/api/netaudit/expectations?workspace_id=ws", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("none is an empty array, not null: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(h.ListExpectations, http.MethodGet, "/", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no workspace = %d", rec.Code)
	}
}

func TestNetauditHandlers_SetExpectation(t *testing.T) {
	exps := &fakeExpectations{}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithExpectations(exps)
	body := `{"workspace_id":"ws","device":"10.0.0.5","ports":[631],"note":"printer"}`

	if rec := do(h.SetExpectation, http.MethodPut, "/", body); rec.Code != http.StatusOK || exps.gotSet.Device != "10.0.0.5" || exps.gotSet.Ports[0] != 631 {
		t.Errorf("set = %d %s (%+v)", rec.Code, rec.Body.String(), exps.gotSet)
	}
	if rec := do(h.SetExpectation, http.MethodPut, "/", `{"workspace_id":"ws","device":"10.0.0.5","ports":[]}`); rec.Code != http.StatusOK {
		t.Errorf("an empty list is allowed: %d", rec.Code)
	}
	if rec := do(h.SetExpectation, http.MethodPut, "/", `{"workspace_id":"ws"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a request without the device = %d", rec.Code)
	}
	for err, want := range map[error]int{netaudit.ErrBadExpectation: http.StatusBadRequest, context.Canceled: http.StatusInternalServerError} {
		exps.setErr = err
		if rec := do(h.SetExpectation, http.MethodPut, "/", body); rec.Code != want {
			t.Errorf("%v = %d, want %d", err, rec.Code, want)
		}
	}
}

func TestNetauditHandlers_RemoveExpectation(t *testing.T) {
	exps := &fakeExpectations{}
	h := NewNetauditHandlers(nil, nil, nil, nil).WithExpectations(exps)
	const url = "/admin/api/netaudit/expectations?workspace_id=ws&device=10.0.0.5"
	if rec := do(h.RemoveExpectation, http.MethodDelete, url, ""); rec.Code != http.StatusNotFound {
		t.Errorf("nothing to remove = %d", rec.Code)
	}
	exps.removeOK = true
	if rec := do(h.RemoveExpectation, http.MethodDelete, url, ""); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d", rec.Code)
	}
	if rec := do(h.RemoveExpectation, http.MethodDelete, "/?workspace_id=ws", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no device = %d", rec.Code)
	}
}

func TestNetauditHandlers_ExpectationsWithoutAServiceAre404(t *testing.T) {
	h := NewNetauditHandlers(nil, nil, nil, nil)
	if rec := do(h.ListExpectations, http.MethodGet, "/?workspace_id=ws", ""); rec.Code != http.StatusNotFound {
		t.Errorf("list = %d", rec.Code)
	}
}
