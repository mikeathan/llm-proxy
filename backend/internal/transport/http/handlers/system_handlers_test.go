package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
)

func TestAdminWipeoutHandler_Error(t *testing.T) {
	admin := &mocks.MockAdminService{
		WipeoutFunc: func() (storage.WipeoutResult, error) {
			return storage.WipeoutResult{}, errors.New("boom")
		},
	}
	h := NewSystemHandlers(admin, &mocks.MockLogger{}, &buildinfo.Info{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/system/wipeout", nil)
	rr := httptest.NewRecorder()
	h.AdminWipeoutHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestAdminWipeoutHandler_SuccessStopsProcess(t *testing.T) {
	orig := shutdownAfterResponse
	exitCalled := make(chan struct{}, 1)
	shutdownAfterResponse = func() { exitCalled <- struct{}{} }
	t.Cleanup(func() { shutdownAfterResponse = orig })

	admin := &mocks.MockAdminService{
		WipeoutFunc: func() (storage.WipeoutResult, error) {
			return storage.WipeoutResult{RootDir: "/r", WorkspacesDir: "/w"}, nil
		},
	}
	h := NewSystemHandlers(admin, &mocks.MockLogger{}, &buildinfo.Info{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/system/wipeout", nil)
	rr := httptest.NewRecorder()
	h.AdminWipeoutHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	select {
	case <-exitCalled:
	case <-time.After(time.Second):
		t.Fatal("expected the process-shutdown to be scheduled")
	}
}
