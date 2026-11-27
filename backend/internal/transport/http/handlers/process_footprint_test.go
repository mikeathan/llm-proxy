package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/testing/mocks"
)

func footprintRequest(t *testing.T, active *llm.ActiveModelInfo) (modelPID int, body metrics.Footprint) {
	t.Helper()
	mgr := &mocks.MockManager{ActiveInfoFunc: func() *llm.ActiveModelInfo { return active }}
	admin := &mocks.MockAdminService{FootprintFunc: func(pid int) metrics.Footprint {
		modelPID = pid
		return metrics.Footprint{Processes: []metrics.ProcessFootprint{{Role: metrics.RoleService, PID: 1}}}
	}}
	h := NewProcessHandlers(mgr, admin, logging.NewNopLogger())
	rec := httptest.NewRecorder()
	h.AdminFootprintHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/api/metrics/footprint", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return modelPID, body
}

func TestAdminFootprint_PassesTheRunningModelsPID(t *testing.T) {
	pid, body := footprintRequest(t, &llm.ActiveModelInfo{PID: 4242})
	if pid != 4242 || len(body.Processes) != 1 {
		t.Fatalf("model pid %d, body %+v", pid, body)
	}
}

func TestAdminFootprint_NoModelRunningPassesZero(t *testing.T) {
	if pid, _ := footprintRequest(t, nil); pid != 0 {
		t.Fatalf("model pid %d, want 0 when no model runs", pid)
	}
}
