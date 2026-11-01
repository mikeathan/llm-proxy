package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
)

type fakeOutsideControl struct {
	on     bool
	url    string
	setErr error
}

func (f *fakeOutsideControl) Enabled() bool      { return f.on }
func (f *fakeOutsideControl) URL() string        { return f.url }
func (f *fakeOutsideControl) DefaultURL() string { return "https://default.example" }
func (f *fakeOutsideControl) Set(on bool) error  { f.on = on; return nil }
func (f *fakeOutsideControl) SetURL(u string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.url = u
	return nil
}

type fakeOutsideStatus struct{}

func (fakeOutsideStatus) Status() netaudit.ExposureStatus {
	return netaudit.ExposureStatus{At: time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC), Outcome: netaudit.OutcomeCannotLook, Reason: netaudit.ReasonPrivate}
}

func TestOutsideLookupHandlers_GetShowsSwitchServiceAndTheLastLook(t *testing.T) {
	h := NewNetauditOutsideLookupHandlers(&fakeOutsideControl{on: true, url: "https://svc.example"}, fakeOutsideStatus{})
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/outside-lookup", "")
	for _, want := range []string{`"enabled":true`, `"url":"https://svc.example"`, `"default_url":"https://default.example"`, `"outcome":"cannot_look"`, `"reason":"private_address"`} {
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, rec.Body.String())
		}
	}
}

func TestOutsideLookupHandlers_PutChangesOnlyWhatIsSent(t *testing.T) {
	c := &fakeOutsideControl{url: "https://svc.example"}
	h := NewNetauditOutsideLookupHandlers(c, fakeOutsideStatus{})
	if rec := do(h.Put, http.MethodPut, "/x", `{"enabled":true}`); rec.Code != http.StatusOK || !c.on || c.url != "https://svc.example" {
		t.Errorf("enabled only: %d on=%v url=%q", rec.Code, c.on, c.url)
	}
	if rec := do(h.Put, http.MethodPut, "/x", `{"url":"https://other.example"}`); rec.Code != http.StatusOK || !c.on || c.url != "https://other.example" {
		t.Errorf("url only: %d on=%v url=%q", rec.Code, c.on, c.url)
	}
	if rec := do(h.Put, http.MethodPut, "/x", `{"url":""}`); rec.Code != http.StatusOK || c.url != "" {
		t.Errorf("an empty url resets: %d %q", rec.Code, c.url)
	}
}

func TestOutsideLookupHandlers_ABadAddressIsRefusedAndChangesNothing(t *testing.T) {
	c := &fakeOutsideControl{url: "https://svc.example", setErr: errors.New("the service address must start with https://")}
	h := NewNetauditOutsideLookupHandlers(c, fakeOutsideStatus{})
	rec := do(h.Put, http.MethodPut, "/x", `{"enabled":true,"url":"http://x"}`)
	if rec.Code != http.StatusBadRequest || c.on {
		t.Errorf("a refused address must stop the whole change: %d on=%v", rec.Code, c.on)
	}
	if rec := do(h.Put, http.MethodPut, "/x", `nope`); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed body must be refused, got %d", rec.Code)
	}
}
