package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

type fakeRouterConn struct {
	err    error
	key    string
	target routerTarget
	pinned string
}

func (f *fakeRouterConn) View() routerconn.View {
	return routerconn.View{Address: f.target.Address, User: f.target.User, KeySaved: f.key != ""}
}
func (f *fakeRouterConn) SetTarget(a, u string) error { f.target = routerTarget{a, u}; return f.err }
func (f *fakeRouterConn) SetKey(k string) error       { f.key = k; return f.err }
func (f *fakeRouterConn) ClearKey() error             { f.key = ""; return f.err }
func (f *fakeRouterConn) Probe(context.Context) (string, error) {
	return "SHA256:abc", f.err
}
func (f *fakeRouterConn) Pin(_ context.Context, c string) error { f.pinned = c; return f.err }
func (f *fakeRouterConn) Read(context.Context) (*models.RouterConnections, error) {
	return &models.RouterConnections{}, f.err
}

func routerConnCall(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h(w, r)
	return w
}

func TestTheRouterKeyIsSavedAndNeverAnsweredBack(t *testing.T) {
	f := &fakeRouterConn{}
	h := NewNetauditRouterConnHandlers(func() RouterConnControl { return f })
	w := routerConnCall(h.PutKey, `{"key":"-----BEGIN OPENSSH PRIVATE KEY-----SECRET"}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "SECRET") || !strings.Contains(w.Body.String(), `"key_saved":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := routerConnCall(h.Get, ""); strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("get: %s", w.Body)
	}
}

func TestEachStepReachesTheControlAndAnswersTheFingerprintAndTheSetup(t *testing.T) {
	f := &fakeRouterConn{}
	h := NewNetauditRouterConnHandlers(func() RouterConnControl { return f })
	if w := routerConnCall(h.PutTarget, `{"address":"192.168.50.1","user":"admin"}`); w.Code != http.StatusOK || f.target.User != "admin" {
		t.Errorf("target: %d %s", w.Code, w.Body)
	}
	if w := routerConnCall(h.Fingerprint, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "SHA256:abc") {
		t.Errorf("fingerprint: %d %s", w.Code, w.Body)
	}
	if w := routerConnCall(h.Pin, `{"fingerprint":"SHA256:abc"}`); w.Code != http.StatusOK || f.pinned != "SHA256:abc" {
		t.Errorf("pin: %d", w.Code)
	}
	if w := routerConnCall(h.Read, ""); w.Code != http.StatusOK {
		t.Errorf("read: %d", w.Code)
	}
	if w := routerConnCall(h.DeleteKey, ""); w.Code != http.StatusOK {
		t.Errorf("delete: %d", w.Code)
	}
}

func TestARefusalHasItsOwnStatusAndAForeignErrorIsNotPassedOn(t *testing.T) {
	cases := []struct {
		err  error
		code int
		said string
	}{
		{routerconn.ErrBusy, http.StatusConflict, "just read"},
		{routerconn.ErrNotSet, http.StatusBadRequest, "set the router"},
		{routerssh.ErrNotPrivate, http.StatusBadRequest, "on this network"},
		{routerssh.ErrHostKeyChanged, http.StatusBadGateway, "not the one you trusted"},
		{errors.New("dial tcp 192.168.50.1:22: banner SECRET-FROM-SERVER"), http.StatusBadGateway, "could not be reached"},
	}
	for _, c := range cases {
		h := NewNetauditRouterConnHandlers(func() RouterConnControl { return &fakeRouterConn{err: c.err} })
		w := routerConnCall(h.Read, "")
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.said) || strings.Contains(w.Body.String(), "SECRET-FROM-SERVER") {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
}

func TestBeforeTheServicesAreWiredEveryRouteSaysSo(t *testing.T) {
	h := NewNetauditRouterConnHandlers(func() RouterConnControl { return nil })
	for name, fn := range map[string]http.HandlerFunc{"get": h.Get, "read": h.Read, "pin": h.Pin} {
		if w := routerConnCall(fn, `{}`); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
}
