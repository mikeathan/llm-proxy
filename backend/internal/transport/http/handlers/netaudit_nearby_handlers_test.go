package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nearby"
)

type fakeNearbyControl struct{ on bool }

func (f *fakeNearbyControl) Set(v bool) error { f.on = v; return nil }

type fakeNearbyLooks struct{ err error }

func (f *fakeNearbyLooks) Request() error { return f.err }
func (f *fakeNearbyLooks) View() (netaudit.NearbyView, error) {
	return netaudit.NearbyView{Enabled: true, Devices: []netaudit.NearbyDeviceView{}}, nil
}

func callNearby(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestNearbyHandlers_PutNeedsTheSwitchValueAndSetsIt(t *testing.T) {
	ctl := &fakeNearbyControl{}
	h := NewNetauditNearbyHandlers(ctl, &fakeNearbyLooks{})
	if rec := callNearby(h.Put, http.MethodPut, `{}`); rec.Code != http.StatusBadRequest || ctl.on {
		t.Fatalf("%d", rec.Code)
	}
	if rec := callNearby(h.Put, http.MethodPut, `{"enabled":true}`); rec.Code != http.StatusOK || !ctl.on {
		t.Fatalf("%d", rec.Code)
	}
}

func TestNearbyHandlers_LookRefusalsAreSpecificAndAcceptedIs202(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{{nil, 202}, {nearby.ErrOff, 409}, {nearby.ErrBusy, 409}, {nearby.ErrTooSoon, 429}} {
		h := NewNetauditNearbyHandlers(&fakeNearbyControl{}, &fakeNearbyLooks{err: tc.err})
		if rec := callNearby(h.Look, http.MethodPost, ``); rec.Code != tc.code {
			t.Errorf("%v: %d, want %d", tc.err, rec.Code, tc.code)
		}
	}
}
