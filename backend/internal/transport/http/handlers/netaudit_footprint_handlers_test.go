package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/footprint"
)

type fakeFootprint struct {
	view     footprint.View
	applyErr error
	checkErr error
	applied  footprint.Command
	key      string
}

func (f *fakeFootprint) View() (footprint.View, error) { return f.view, nil }
func (f *fakeFootprint) Apply(c footprint.Command) error {
	f.applied = c
	return f.applyErr
}
func (f *fakeFootprint) Check(context.Context, string) error { return f.checkErr }
func (f *fakeFootprint) SetKey(k string) error               { f.key = k; return nil }
func (f *fakeFootprint) ClearKey() error                     { f.key = ""; return nil }

func TestFootprint_GetPutAndTheKeyIsNeverAnsweredBack(t *testing.T) {
	f := &fakeFootprint{view: footprint.View{KeySaved: true, Watches: []footprint.WatchView{}}}
	h := NewNetauditFootprintHandlers(f)
	if rec := do(h.Get, http.MethodGet, "/", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"key_saved":true`) {
		t.Errorf("get: %d %s", rec.Code, rec.Body.String())
	}
	rec := do(h.PutKey, http.MethodPut, "/", `{"key":"sekrit-key"}`)
	if rec.Code != http.StatusOK || f.key != "sekrit-key" || strings.Contains(rec.Body.String(), "sekrit") {
		t.Errorf("key: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h.Put, http.MethodPut, "/", `{"action":"add_domain","name":"example.com"}`); rec.Code != http.StatusOK || f.applied.Name != "example.com" {
		t.Errorf("put: %d", rec.Code)
	}
	f.applyErr = errors.New("at most 10 domains can be watched")
	if rec := do(h.Put, http.MethodPut, "/", `{"action":"add_domain","name":"x.com"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at most 10") {
		t.Errorf("refusal must be readable: %d %s", rec.Code, rec.Body.String())
	}
	f.applyErr = footprint.ErrNoSuchWatch
	if rec := do(h.Put, http.MethodPut, "/", `{"action":"remove","label":"domain:x.com"}`); rec.Code != http.StatusNotFound {
		t.Errorf("no such watch = %d", rec.Code)
	}
}

func TestFootprint_CheckStatusOfEachRefusal(t *testing.T) {
	cases := map[error]int{
		nil:                      http.StatusOK,
		footprint.ErrNoSuchWatch: http.StatusNotFound,
		footprint.ErrBusy:        http.StatusConflict,
		&footprint.CooldownError{Remaining: 30 * time.Second}: http.StatusTooManyRequests,
		errors.New("boom"): http.StatusInternalServerError,
	}
	for err, want := range cases {
		h := NewNetauditFootprintHandlers(&fakeFootprint{checkErr: err})
		rec := do(h.Check, http.MethodPost, "/", `{"label":"domain:example.com"}`)
		if rec.Code != want {
			t.Errorf("%v = %d, want %d", err, rec.Code, want)
		}
		if want == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "boom") {
			t.Error("an internal error's text must not reach the page")
		}
	}
}
