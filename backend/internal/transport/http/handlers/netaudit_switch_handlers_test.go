package handlers

import (
	"errors"
	"net/http"
	"testing"
)

type fakeSwitch struct {
	on  bool
	err error
}

func (f *fakeSwitch) Enabled() bool { return f.on }
func (f *fakeSwitch) Set(on bool) error {
	if f.err == nil {
		f.on = on
	}
	return f.err
}

func TestSwitchHandlers_GetAndPutTheSwitch(t *testing.T) {
	f := &fakeSwitch{}
	h := NewNetauditSwitchHandlers(f)
	if rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/weekly-summary", `{"enabled":true}`); rec.Code != http.StatusOK || !f.on || rec.Body.String() == "" {
		t.Errorf("put = %d %s, on = %v", rec.Code, rec.Body.String(), f.on)
	}
	if rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/weekly-summary", ""); rec.Code != http.StatusOK || rec.Body.String() != "{\"enabled\":true}\n" {
		t.Errorf("get = %d %q", rec.Code, rec.Body.String())
	}
}

func TestSwitchHandlers_AFailedSaveIsAServerError(t *testing.T) {
	h := NewNetauditSwitchHandlers(&fakeSwitch{err: errors.New("disk full")})
	if rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/weekly-summary", `{"enabled":true}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d", rec.Code)
	}
}
