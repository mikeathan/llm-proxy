package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/stack"
	api "llm-proxy/internal/transport/http"
)

type stackSwitch struct {
	on     bool
	sets   int
	setErr error
}

func (s *stackSwitch) Enabled() bool { return s.on }
func (s *stackSwitch) Set(e bool) error {
	s.sets++
	if s.setErr != nil {
		return s.setErr
	}
	s.on = e
	return nil
}

type stackRig struct {
	presence, summary, decoys *stackSwitch
	h                         *StackHandlers
	stalled                   []string
}

func newStackRig(t *testing.T) *stackRig {
	t.Helper()
	r := &stackRig{presence: &stackSwitch{}, summary: &stackSwitch{}, decoys: &stackSwitch{}}
	reg, err := stack.New(
		[]stack.Stack{{ID: stack.Network, Title: "Network", Layers: []stack.Layer{{ID: "discover", Title: "Discover"}, {ID: "listen", Title: "Listen"}, {ID: "tell", Title: "Tell"}}}},
		[]stack.Module{
			{ID: "presence", Title: "Presence check", Stack: stack.Network, Layer: "discover", Sends: stack.SendsLAN, Switch: r.presence, Monitors: []string{"presence"}},
			{ID: "decoys", Title: "Decoys", Stack: stack.Network, Layer: "listen", Sends: stack.Listens, StepUp: true, Switch: r.decoys},
			{ID: "summary", Title: "Weekly summary", Stack: stack.Network, Layer: "tell", Sends: stack.SendsMessage, Needs: []string{"presence"}, Switch: r.summary},
		})
	if err != nil {
		t.Fatal(err)
	}
	r.h = NewStackHandlers(reg, func() []string { return r.stalled })
	return r
}

// call runs a handler through a mux, so the path values are real, with the step-up status the test chooses.
func (r *stackRig) call(method, target, body string, stepUp bool) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/stacks", r.h.List)
	mux.HandleFunc("GET /admin/api/stacks/{stack}", r.h.Get)
	mux.HandleFunc("PUT /admin/api/stacks/{stack}/modules/{id}", r.h.PutModule)
	req := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(api.WithStepUp(context.Background(), stepUp))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestStackHandlers_ListAndGetDescribeTheStacks(t *testing.T) {
	r := newStackRig(t)
	r.presence.on = true
	r.stalled = []string{"presence"}
	list := r.call(http.MethodGet, "/admin/api/stacks", "", true)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"network"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var v stack.StackView
	rec := r.call(http.MethodGet, "/admin/api/stacks/network", "", true)
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("get = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if v.Layers[0].ID != "tell" || v.Summary.Stopped != 1 {
		t.Fatalf("view = %+v; want the top layer first and the stalled presence check counted", v)
	}
	if rec := r.call(http.MethodGet, "/admin/api/stacks/mars", "", true); rec.Code != http.StatusNotFound {
		t.Errorf("unknown stack = %d, want 404", rec.Code)
	}
}

func TestStackHandlers_PutSwitchesAModuleAndAnswersWithTheStack(t *testing.T) {
	r := newStackRig(t)
	rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/presence", `{"enabled":true}`, false)
	if rec.Code != http.StatusOK || !r.presence.on {
		t.Fatalf("put = %d %s, on = %v", rec.Code, rec.Body.String(), r.presence.on)
	}
	if !strings.Contains(rec.Body.String(), `"layers"`) {
		t.Errorf("the answer must carry the stack's new state: %s", rec.Body.String())
	}
}

func TestStackHandlers_PutRefusesBadRequests(t *testing.T) {
	r := newStackRig(t)
	for _, c := range []struct {
		target, body string
		want         int
	}{
		{"/admin/api/stacks/mars/modules/presence", `{"enabled":true}`, http.StatusNotFound},
		{"/admin/api/stacks/network/modules/ghost", `{"enabled":true}`, http.StatusNotFound},
		{"/admin/api/stacks/network/modules/presence", `{}`, http.StatusBadRequest},
		{"/admin/api/stacks/network/modules/presence", `{"enabled":"yes"}`, http.StatusBadRequest},
		{"/admin/api/stacks/network/modules/presence", `not json`, http.StatusBadRequest},
	} {
		if rec := r.call(http.MethodPut, c.target, c.body, true); rec.Code != c.want {
			t.Errorf("PUT %s %s = %d, want %d", c.target, c.body, rec.Code, c.want)
		}
	}
	if r.presence.sets != 0 {
		t.Error("a bad request changed a switch")
	}
}

func TestStackHandlers_AModuleThatNeedsThePassphraseIsRefusedWithoutIt(t *testing.T) {
	r := newStackRig(t)
	rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/decoys", `{"enabled":true}`, false)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "reauth_required") {
		t.Fatalf("put without the passphrase = %d %s, want 403 reauth_required (the UI's cue to ask)", rec.Code, rec.Body.String())
	}
	if r.decoys.sets != 0 {
		t.Fatal("the switch was changed although the passphrase was missing")
	}
	if rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/decoys", `{"enabled":true}`, true); rec.Code != http.StatusOK || !r.decoys.on {
		t.Fatalf("with the passphrase = %d, on = %v", rec.Code, r.decoys.on)
	}
}

func TestStackHandlers_SwitchingAModuleOnWhileItsNeedIsOffIsAConflictNamingTheNeed(t *testing.T) {
	r := newStackRig(t)
	rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/summary", `{"enabled":true}`, true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Presence check") {
		t.Fatalf("put = %d %s, want 409 naming the Presence check", rec.Code, rec.Body.String())
	}
}

func TestStackHandlers_SwitchingOffNamesWhatIsLeftBlocked(t *testing.T) {
	r := newStackRig(t)
	r.presence.on, r.summary.on = true, true
	rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/presence", `{"enabled":false}`, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"blocked":["Weekly summary"]`) {
		t.Fatalf("put = %d %s, want 200 with the weekly summary named as blocked", rec.Code, rec.Body.String())
	}
}

func TestStackHandlers_AFailedWriteIsAServerError(t *testing.T) {
	r := newStackRig(t)
	r.presence.setErr = errors.New("disk full")
	if rec := r.call(http.MethodPut, "/admin/api/stacks/network/modules/presence", `{"enabled":true}`, true); rec.Code != http.StatusInternalServerError {
		t.Fatalf("put = %d %s, want 500", rec.Code, rec.Body.String())
	}
}
