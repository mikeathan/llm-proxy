package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/vulnfeed"
)

type stubVulnData struct {
	status   vulnfeed.Status
	startErr error
	started  int
	restart  bool
	canceled int
	cleared  int
	clearErr error
}

func (s *stubVulnData) Status() vulnfeed.Status { return s.status }
func (s *stubVulnData) Start(restart bool) error {
	s.started++
	s.restart = restart
	return s.startErr
}
func (s *stubVulnData) Cancel()      { s.canceled++ }
func (s *stubVulnData) Clear() error { s.cleared++; return s.clearErr }

func TestVulnDataHandlers_StatusIsServedAsJSON(t *testing.T) {
	stub := &stubVulnData{status: vulnfeed.Status{State: vulnfeed.StateRunning, File: "NVD 2025", FilesDone: 2, FilesTotal: 6}}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Get(rec, httptest.NewRequest(http.MethodGet, "/admin/api/netaudit/vulndata", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["state"] != "running" || got["file"] != "NVD 2025" || got["files_total"] != float64(6) {
		t.Errorf("body = %s (%v)", rec.Body.String(), err)
	}
}

func TestVulnDataHandlers_DownloadStartsAndAnswersAccepted(t *testing.T) {
	stub := &stubVulnData{status: vulnfeed.Status{State: vulnfeed.StateRunning}}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Download(rec, httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/vulndata/download", nil))
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Code != http.StatusAccepted || stub.started != 1 {
		t.Errorf("code %d, started %d", rec.Code, stub.started)
	}
}

func TestVulnDataHandlers_DownloadAsksToStartAgainOnlyWhenToldTo(t *testing.T) {
	for query, want := range map[string]bool{"": false, "?restart=1": true, "?restart=0": false, "?restart=yes": false} {
		stub := &stubVulnData{}
		NewVulnDataHandlers(stub).Download(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/vulndata/download"+query, nil))
		if stub.restart != want {
			t.Errorf("%q: restart = %v, want %v", query, stub.restart, want)
		}
	}
}

func TestVulnDataHandlers_DownloadWhileRunningIsAConflict(t *testing.T) {
	stub := &stubVulnData{startErr: vulnfeed.ErrBusy}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Download(rec, httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/vulndata/download", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("code = %d, want 409", rec.Code)
	}
}

func TestVulnDataHandlers_CancelStopsTheJob(t *testing.T) {
	stub := &stubVulnData{}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Cancel(rec, httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/vulndata/cancel", nil))
	if rec.Code != http.StatusOK || stub.canceled != 1 {
		t.Errorf("code %d, canceled %d", rec.Code, stub.canceled)
	}
}

func TestVulnDataHandlers_DownloadWhenUpToDateAnswersOKWithoutStarting(t *testing.T) {
	stub := &stubVulnData{startErr: vulnfeed.ErrUpToDate, status: vulnfeed.Status{State: vulnfeed.StateDone}}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Download(rec, httptest.NewRequest(http.MethodPost, "/admin/api/netaudit/vulndata/download", nil))
	var got map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["state"] != "done" {
		t.Errorf("code %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestVulnDataHandlers_ClearDeletesTheDataAndAnswersWithTheStatus(t *testing.T) {
	stub := &stubVulnData{status: vulnfeed.Status{State: vulnfeed.StateIdle, FilesTotal: 6}}
	rec := httptest.NewRecorder()
	NewVulnDataHandlers(stub).Clear(rec, httptest.NewRequest(http.MethodDelete, "/admin/api/netaudit/vulndata", nil))
	var got map[string]any
	if rec.Code != http.StatusOK || stub.cleared != 1 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["state"] != "idle" {
		t.Errorf("code %d, cleared %d, body %s", rec.Code, stub.cleared, rec.Body.String())
	}
	stub.clearErr = errors.New("failed to delete the vulnerability data: read-only")
	rec = httptest.NewRecorder()
	NewVulnDataHandlers(stub).Clear(rec, httptest.NewRequest(http.MethodDelete, "/admin/api/netaudit/vulndata", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "read-only") {
		t.Errorf("a failed delete = %d %s", rec.Code, rec.Body.String())
	}
}
