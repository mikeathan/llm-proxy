package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/process"
	"llm-proxy/internal/testing/mocks"
)

// The stop endpoint is for the model servers the Models page lists. It used to signal any process id the service user
// could signal (plan admin-api-authentication, E4); it now refuses one the page could not have listed.
func killRequest(h *ProcessHandlers, pid string) int {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/runtime/processes/"+pid+"/stop", nil)
	req.SetPathValue("pid", pid)
	rec := httptest.NewRecorder()
	h.AdminProcessKillHandler(rec, req)
	return rec.Code
}

func newKillHandlers(listed []process.Info, killed *[]int) *ProcessHandlers {
	h := NewProcessHandlers(&mocks.MockManager{}, &mocks.MockAdminService{}, logging.NewNopLogger())
	h.listModelServers = func(int) ([]process.Info, error) { return listed, nil }
	h.killProcess = func(pid int) error { *killed = append(*killed, pid); return nil }
	return h
}

func TestAdminProcessKill_OnlyStopsAModelServerItLists(t *testing.T) {
	var killed []int
	h := newKillHandlers([]process.Info{{PID: 4242, Binary: "llama-server"}}, &killed)

	if code := killRequest(h, "4242"); code != http.StatusOK || len(killed) != 1 || killed[0] != 4242 {
		t.Errorf("a listed model server: code %d, killed %v", code, killed)
	}
	killed = nil
	for _, pid := range []string{"1", "99999", "4243"} {
		if code := killRequest(h, pid); code != http.StatusForbidden {
			t.Errorf("pid %s is not a listed model server: code %d, want 403", pid, code)
		}
	}
	if len(killed) != 0 {
		t.Errorf("signalled processes it does not own: %v", killed)
	}
}

func TestAdminProcessKill_RefusesWhenTheListCannotBeRead(t *testing.T) {
	var killed []int
	h := newKillHandlers(nil, &killed)
	h.listModelServers = func(int) ([]process.Info, error) { return nil, http.ErrAbortHandler }
	if code := killRequest(h, "4242"); code == http.StatusOK || len(killed) != 0 {
		t.Errorf("code %d, killed %v: it must fail closed", code, killed)
	}
}

func TestAdminProcessKill_BadPID(t *testing.T) {
	var killed []int
	if code := killRequest(newKillHandlers(nil, &killed), "abc"); code != http.StatusBadRequest {
		t.Errorf("code %d, want 400", code)
	}
}
