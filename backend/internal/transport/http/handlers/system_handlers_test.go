package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
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
	exitCalled := make(chan int, 1)
	shutdownAfterResponse = func(code int) { exitCalled <- code }
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
	case code := <-exitCalled:
		if code != 0 {
			t.Fatalf("wipeout must exit 0 so the supervisor does not relaunch it, got %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the process-shutdown to be scheduled")
	}
}

// TestAdminRestartHandler_ExitsNonZeroSoSupervisorRelaunches guards the
// "Restart now never came back" bug: systemd (Restart=on-failure) and launchd
// (KeepAlive SuccessfulExit=false) treat exit 0 as an intentional stop and do
// not relaunch, so a restart must exit non-zero.
func TestAdminRestartHandler_ExitsNonZeroSoSupervisorRelaunches(t *testing.T) {
	orig := shutdownAfterResponse
	exitCalled := make(chan int, 1)
	shutdownAfterResponse = func(code int) { exitCalled <- code }
	t.Cleanup(func() { shutdownAfterResponse = orig })

	h := NewSystemHandlers(&mocks.MockAdminService{}, &mocks.MockLogger{}, &buildinfo.Info{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/system/restart", nil)
	rr := httptest.NewRecorder()
	h.AdminRestartHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	select {
	case code := <-exitCalled:
		if code == 0 {
			t.Fatal("restart must exit non-zero; exit 0 is treated as a clean stop and never relaunched")
		}
	case <-time.After(time.Second):
		t.Fatal("expected the process-shutdown to be scheduled")
	}
}

// TestAdminConfigUpdateHandler_SearchValidationIsBadRequest verifies an invalid
// search config surfaces as 400 (boundary validation), not 500.
func TestAdminConfigUpdateHandler_SearchValidationIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"invalid provider", fmt.Errorf("%w: %q", models.ErrInvalidSearchProvider, "bogus")},
		{"max results out of range", fmt.Errorf("%w: %d", models.ErrSearchMaxResultsOutOfRange, 99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admin := &mocks.MockAdminService{
				ApplySystemUpdateFunc: func(context.Context, models.SystemUpdatePayload) error { return tt.err },
			}
			h := NewSystemHandlers(admin, &mocks.MockLogger{}, &buildinfo.Info{})

			req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader(`{"search":{"provider":"bogus","max_results":99}}`))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.AdminConfigUpdateHandler(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for a search-config validation error, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}
