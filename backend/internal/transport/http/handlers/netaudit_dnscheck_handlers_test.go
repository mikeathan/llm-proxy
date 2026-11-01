package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
)

type fakeDNSControl struct {
	view     netaudit.DNSCheckView
	interval int
	names    []string
	url      *string
	server   *string
	err      error
}

func (f *fakeDNSControl) View() netaudit.DNSCheckView { return f.view }
func (f *fakeDNSControl) SetEnabled(on bool) error    { f.view.Enabled = on; return nil }
func (f *fakeDNSControl) SetInterval(m int) error     { f.interval = m; return f.err }
func (f *fakeDNSControl) SetNames(n []string) error   { f.names = n; return f.err }
func (f *fakeDNSControl) SetServer(s string) error {
	f.server = &s
	return f.err
}
func (f *fakeDNSControl) SetResolverURL(u string) error {
	f.url = &u
	return f.err
}

type fakeDNSReport struct{ err error }

func (fakeDNSReport) Status() netaudit.DNSStatus {
	return netaudit.DNSStatus{LastRun: time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC), Problem: netaudit.DNSProblemInternetOff}
}

func (f fakeDNSReport) Summary() (netaudit.DNSSummary, error) {
	return netaudit.DNSSummary{Comparisons: 7, Matches: 6, Differs: 1, Differences: nil}, f.err
}

func TestDNSCheckHandlers_GetShowsSettingsStatusAndTheSummaryToBringBack(t *testing.T) {
	c := &fakeDNSControl{view: netaudit.DNSCheckView{Enabled: true, Names: []string{"dns.google"}}}
	h := NewNetauditDNSCheckHandlers(c, fakeDNSReport{})
	rec := do(h.Get, http.MethodGet, "/admin/api/netaudit/dns-check", "")
	for _, want := range []string{`"enabled":true`, `"names":["dns.google"]`, `"problem":"internet_off"`, `"comparisons":7`, `"differs":1`} {
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q in %d %s", want, rec.Code, rec.Body.String())
		}
	}
	broken := NewNetauditDNSCheckHandlers(c, fakeDNSReport{err: errors.New("disk")})
	if rec = do(broken.Get, http.MethodGet, "/x", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("an unreadable log is an error, not an empty summary: %d", rec.Code)
	}
}

func TestDNSCheckHandlers_PutChangesOnlyWhatIsSent(t *testing.T) {
	c := &fakeDNSControl{}
	h := NewNetauditDNSCheckHandlers(c, fakeDNSReport{})
	rec := do(h.Put, http.MethodPut, "/x", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !c.view.Enabled || c.interval != 0 || c.names != nil || c.url != nil {
		t.Errorf("enabled only: %d %+v", rec.Code, c)
	}
	rec = do(h.Put, http.MethodPut, "/x", `{"interval_minutes":30,"names":["a.example"],"resolver_url":"https://r.example/q"}`)
	if rec.Code != http.StatusOK || c.interval != 30 || len(c.names) != 1 || c.url == nil || *c.url != "https://r.example/q" {
		t.Errorf("the rest: %d %+v", rec.Code, c)
	}
	rec = do(h.Put, http.MethodPut, "/x", `{"server":"192.168.1.53"}`)
	if rec.Code != http.StatusOK || c.server == nil || *c.server != "192.168.1.53" {
		t.Errorf("the router's DNS address: %d %v", rec.Code, c.server)
	}
	if rec = do(h.Put, http.MethodPut, "/x", `{"names":[]}`); rec.Code != http.StatusOK || c.names == nil || len(c.names) != 0 {
		t.Errorf("an empty list resets to the defaults and is passed on: %d %v", rec.Code, c.names)
	}
}

// A refused setting is a 400 and the switch is not turned on in the same request.
func TestDNSCheckHandlers_APutWithABadSettingRefusesAndSwitchesNothingOn(t *testing.T) {
	c := &fakeDNSControl{err: errors.New("bad")}
	h := NewNetauditDNSCheckHandlers(c, fakeDNSReport{})
	rec := do(h.Put, http.MethodPut, "/x", `{"enabled":true,"names":["bad name"]}`)
	if rec.Code != http.StatusBadRequest || c.view.Enabled {
		t.Errorf("%d enabled=%v", rec.Code, c.view.Enabled)
	}
	if rec = do(h.Put, http.MethodPut, "/x", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", rec.Code)
	}
}
