package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

type fakeLockdown struct {
	state      models.Lockdown
	keep       string
	revokedAll bool
}

func (f *fakeLockdown) State() models.Lockdown { return f.state }
func (f *fakeLockdown) Engage(keep string, revokeTokens bool) (models.Lockdown, error) {
	f.keep, f.revokedAll = keep, revokeTokens
	f.state = models.Lockdown{On: true, SignedOut: 2}
	return f.state, nil
}
func (f *fakeLockdown) Release() (models.Lockdown, error) {
	f.state = models.Lockdown{}
	return f.state, nil
}

func TestLockdownHandlers_EngageKeepsThisSessionAndReleaseLifts(t *testing.T) {
	f := &fakeLockdown{}
	h := NewLockdownHandlers(f)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/lockdown", strings.NewReader(`{"revoke_tokens":true}`))
	req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: "this-browser"})
	w := httptest.NewRecorder()
	h.Engage(w, req)
	var got models.Lockdown
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || !got.On || f.keep != "this-browser" || !f.revokedAll {
		t.Fatalf("status %d state %+v keep %q tokens %v", w.Code, got, f.keep, f.revokedAll)
	}
	w = httptest.NewRecorder()
	h.Release(w, httptest.NewRequest(http.MethodDelete, "/admin/api/lockdown", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || got.On {
		t.Fatalf("release: status %d state %+v", w.Code, got)
	}
	w = httptest.NewRecorder()
	h.Engage(w, httptest.NewRequest(http.MethodPost, "/admin/api/lockdown", strings.NewReader(`not json`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("a body that is not the switch's is refused, got %d", w.Code)
	}
}
