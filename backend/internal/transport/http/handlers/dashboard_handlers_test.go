package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/dashboard"
	"llm-proxy/models"
)

type memDashboardStore struct{ d models.Dashboard }

func (m *memDashboardStore) Read() (models.Dashboard, error) { return m.d, nil }
func (m *memDashboardStore) Write(d models.Dashboard) error  { m.d = d; return nil }
func (m *memDashboardStore) Delete() error                   { m.d = models.Dashboard{}; return nil }

func newDashboardHandlers() *DashboardHandlers {
	return NewDashboardHandlers(dashboard.New(&memDashboardStore{}))
}

func serve(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/admin/api/dashboard", strings.NewReader(body)))
	return rec
}

func TestDashboard_NothingStoredAnswersAnEmptyListNotNull(t *testing.T) {
	rec := serve(newDashboardHandlers().Get, http.MethodGet, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"boards":[]`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDashboard_PutThenGetReturnsTheLayout(t *testing.T) {
	h := newDashboardHandlers()
	put := serve(h.Put, http.MethodPut, `{"boards":[{"id":"home","name":"Home","cards":[{"id":"radar","cols":8,"rows":2}]}]}`)
	if put.Code != http.StatusOK {
		t.Fatalf("put: %d %s", put.Code, put.Body.String())
	}
	got := serve(h.Get, http.MethodGet, "")
	if !strings.Contains(got.Body.String(), `"cols":8`) {
		t.Fatalf("get after put: %s", got.Body.String())
	}
}

func TestDashboard_PutRefusesAnInvalidLayoutWithTheReason(t *testing.T) {
	rec := serve(newDashboardHandlers().Put, http.MethodPut, `{"boards":[{"id":"home","name":"Home","cards":[{"id":"x","cols":5,"rows":1}]}]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "width of 5") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDashboard_PutRefusesBrokenJSONAndAnOversizeBody(t *testing.T) {
	h := newDashboardHandlers()
	if rec := serve(h.Put, http.MethodPut, "{nope"); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken json: %d", rec.Code)
	}
	if rec := serve(h.Put, http.MethodPut, strings.Repeat(" ", maxDashboardBody+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: %d", rec.Code)
	}
}

func TestDashboard_DeleteForgetsTheLayout(t *testing.T) {
	h := newDashboardHandlers()
	serve(h.Put, http.MethodPut, `{"boards":[{"id":"home","name":"Home","cards":[]}]}`)
	if rec := serve(h.Delete, http.MethodDelete, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if got := serve(h.Get, http.MethodGet, ""); !strings.Contains(got.Body.String(), `"boards":[]`) {
		t.Fatalf("after delete: %s", got.Body.String())
	}
}
