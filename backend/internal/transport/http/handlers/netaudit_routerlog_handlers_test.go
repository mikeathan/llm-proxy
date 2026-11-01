package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
)

type fakeRouterLogRuntime struct{ poked int }

func (f *fakeRouterLogRuntime) Status() routerlog.Status {
	return routerlog.Status{Listening: true, LastHeard: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
}
func (f *fakeRouterLogRuntime) Poke() { f.poked++ }

func TestRouterLogHandlers_GetShowsSwitchPortAndWhenTheRouterWasHeard(t *testing.T) {
	h := NewNetauditRouterLogHandlers(&fakeSwitch{on: true}, &fakeRouterLogRuntime{})
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/router-log", "")
	for _, want := range []string{`"enabled":true`, `"listening":true`, `"port":5514`, `"last_heard":"2026-11-01T09:00:00Z"`} {
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, rec.Body.String())
		}
	}
}

func TestRouterLogHandlers_PutSwitchesAndWakesTheListener(t *testing.T) {
	control, runtime := &fakeSwitch{}, &fakeRouterLogRuntime{}
	h := NewNetauditRouterLogHandlers(control, runtime)
	rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/router-log", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !control.on || runtime.poked != 1 {
		t.Errorf("put = %d on=%v poked=%d", rec.Code, control.on, runtime.poked)
	}
	if rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/router-log", `nope`); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed body must be refused, got %d", rec.Code)
	}
}
