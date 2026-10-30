package handlers

import (
	"net/http"
	"strings"
	"testing"
)

type fakeDiscoveryControl struct{ on bool }

func (f *fakeDiscoveryControl) Enabled() bool     { return f.on }
func (f *fakeDiscoveryControl) Set(on bool) error { f.on = on; return nil }

type fakeDiscoveryStatus string

func (f fakeDiscoveryStatus) Discovery6Problem() string { return string(f) }

func TestDiscovery6Handlers_GetShowsTheSwitchAndTheProblemCode(t *testing.T) {
	h := NewNetauditDiscovery6Handlers(&fakeDiscoveryControl{on: true}, fakeDiscoveryStatus("not_permitted"))
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/ipv6-discovery", "")
	body := rec.Body.String()
	for _, want := range []string{`"enabled":true`, `"problem":"not_permitted"`} {
		if rec.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, body)
		}
	}
}

func TestDiscovery6Handlers_PutSwitchesAndAnEmptyProblemIsLeftOut(t *testing.T) {
	control := &fakeDiscoveryControl{}
	h := NewNetauditDiscovery6Handlers(control, fakeDiscoveryStatus(""))
	rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/ipv6-discovery", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !control.on || strings.Contains(rec.Body.String(), "problem") {
		t.Errorf("put = %d on=%v %s", rec.Code, control.on, rec.Body.String())
	}
	if rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/ipv6-discovery", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d", rec.Code)
	}
}
