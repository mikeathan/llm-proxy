package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func heartbeatHandlers(t *testing.T, d *testDispatcher) (*DispatcherHandlers, *persistence.WorkspaceManager) {
	t.Helper()
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	d.mgr = mgr
	return NewDispatcherHandlers(d, NewWorkspaceService(mgr), logging.NewNopLogger()), mgr
}

func heartbeatRequest(method, body string) *http.Request {
	req := httptest.NewRequest(method, "/admin/api/dispatcher/workspaces/ws/heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	return req
}

func TestGetHeartbeat_ReturnsTheDispatchersState(t *testing.T) {
	d := &testDispatcher{heartbeat: models.HeartbeatState{Lane: "cloud", HasChecks: true, Config: models.HeartbeatConfig{Enabled: true, Every: "1h"}}}
	h, _ := heartbeatHandlers(t, d)
	rr := httptest.NewRecorder()
	h.GetHeartbeat(rr, heartbeatRequest("GET", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var got models.HeartbeatState
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Lane != "cloud" || !got.HasChecks || got.Config.Every != "1h" {
		t.Errorf("state = %+v", got)
	}
}

func TestPutHeartbeat_SavesIntoTheWorkspaceConfigAndSchedulesAtOnce(t *testing.T) {
	d := &testDispatcher{}
	h, mgr := heartbeatHandlers(t, d)
	if err := mgr.WriteConfig("ws", &models.WorkspaceConfig{Model: "qwen", Temperature: 0.3}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.PutHeartbeat(rr, heartbeatRequest("PUT", `{"enabled":true,"every":"15m","model":"gpt-5","notify":{"connector":"my-telegram"}}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	cfg, _ := mgr.ReadConfig("ws")
	if cfg.Heartbeat == nil || !cfg.Heartbeat.Enabled || cfg.Heartbeat.Every != "15m" || cfg.Heartbeat.Notify == nil || cfg.Heartbeat.Notify.Connector != "my-telegram" {
		t.Errorf("saved heartbeat = %+v", cfg.Heartbeat)
	}
	if cfg.Model != "qwen" || cfg.Temperature != 0.3 {
		t.Errorf("the rest of the workspace config changed: %+v", cfg)
	}
	if len(d.registered) != 1 || d.registered[0] != "ws/"+models.HeartbeatAutomationName {
		t.Errorf("registered = %v, want the heartbeat scheduled immediately", d.registered)
	}

	rr = httptest.NewRecorder()
	h.PutHeartbeat(rr, heartbeatRequest("PUT", `{"enabled":false,"every":"15m"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if len(d.unregistered) != 1 || d.unregistered[0] != "ws/"+models.HeartbeatAutomationName {
		t.Errorf("unregistered = %v, want the heartbeat unscheduled when switched off", d.unregistered)
	}
}

func TestPutHeartbeat_RejectsBadInputWithoutChangingAnything(t *testing.T) {
	for name, body := range map[string]string{
		"too frequent":             `{"enabled":true,"every":"5s"}`,
		"not a duration":           `{"enabled":true,"every":"often"}`,
		"notify without connector": `{"enabled":true,"notify":{"dedup":true}}`,
		"not json":                 `{`,
	} {
		t.Run(name, func(t *testing.T) {
			d := &testDispatcher{}
			h, mgr := heartbeatHandlers(t, d)
			rr := httptest.NewRecorder()
			h.PutHeartbeat(rr, heartbeatRequest("PUT", body))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
			}
			if cfg, _ := mgr.ReadConfig("ws"); cfg.Heartbeat != nil {
				t.Errorf("a rejected request was saved: %+v", cfg.Heartbeat)
			}
			if len(d.registered)+len(d.unregistered) != 0 {
				t.Error("a rejected request touched the schedule")
			}
		})
	}
}
