package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/models"
)

type fakeDecoyControl struct{ on bool }

func (f *fakeDecoyControl) Enabled() bool     { return f.on }
func (f *fakeDecoyControl) Set(on bool) error { f.on = on; return nil }

type fakeDecoyRuntime struct{ poked int }

func (f *fakeDecoyRuntime) Status() decoy.Status {
	return decoy.Status{Listening: []int{2222}, Problem: "port 8023 is in use or not allowed"}
}
func (f *fakeDecoyRuntime) Poke() { f.poked++ }

type fakeTouches struct{ items []models.DecoyTouch }

func (f fakeTouches) Read() ([]models.DecoyTouch, error) { return f.items, nil }

func TestDecoyHandlers_GetShowsSwitchPortsTroubleAndTouches(t *testing.T) {
	h := NewNetauditDecoyHandlers(&fakeDecoyControl{on: true}, &fakeDecoyRuntime{}, fakeTouches{items: []models.DecoyTouch{{Source: "192.168.1.77", Port: 2222, Count: 2, First: time.Now(), Last: time.Now()}}})
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/decoys", "")
	body := rec.Body.String()
	for _, want := range []string{`"enabled":true`, `"listening":[2222]`, "in use", `"SSH login"`, "192.168.1.77"} {
		if rec.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, body)
		}
	}
}

func TestDecoyHandlers_PutSwitchesAndWakesTheListeners(t *testing.T) {
	control, runtime := &fakeDecoyControl{}, &fakeDecoyRuntime{}
	h := NewNetauditDecoyHandlers(control, runtime, fakeTouches{})
	rec := do(h.Put, http.MethodPut, "/admin/api/netaudit/decoys", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !control.on || runtime.poked != 1 {
		t.Errorf("put = %d on=%v poked=%d", rec.Code, control.on, runtime.poked)
	}
	if !strings.Contains(rec.Body.String(), `"touches":[]`) {
		t.Errorf("an empty list is [] not null: %s", rec.Body.String())
	}
}
