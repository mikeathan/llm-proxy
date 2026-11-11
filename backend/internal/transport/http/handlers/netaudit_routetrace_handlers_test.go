package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/traceroute"
	"llm-proxy/models"
)

type fakeTracer struct {
	started netip.Addr
	repeat  bool
	err     error
}

func (f *fakeTracer) Start(a netip.Addr) error {
	f.started = a
	return f.err
}
func (f *fakeTracer) StartRepeat(a netip.Addr) error {
	f.started, f.repeat = a, true
	return f.err
}
func (f *fakeTracer) Traces() []models.RouteTrace {
	return []models.RouteTrace{{Address: "1.1.1.1", State: models.RouteRunning, Hops: []models.RouteHop{}}}
}

func TestARouteTraceStartsForOneAddressAndAnswersTheKeptTraces(t *testing.T) {
	f := &fakeTracer{}
	h := NewNetauditRouteTraceHandlers(f)
	w := httptest.NewRecorder()
	h.Start(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"address":" 1.1.1.1 "}`)))
	if w.Code != http.StatusAccepted || f.started != netip.MustParseAddr("1.1.1.1") || !strings.Contains(w.Body.String(), `"state":"running"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("the answer is not marked as JSON")
	}
}

// Asking for the repeat starts the rounds; asking for nothing starts a plain trace.
func TestARouteTraceStartsTheRepeatOnlyWhenAskedTo(t *testing.T) {
	for body, want := range map[string]bool{`{"address":"1.1.1.1"}`: false, `{"address":"1.1.1.1","repeat":true}`: true} {
		f := &fakeTracer{}
		w := httptest.NewRecorder()
		NewNetauditRouteTraceHandlers(f).Start(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if w.Code != http.StatusAccepted || f.repeat != want {
			t.Errorf("%s: %d repeat=%v", body, w.Code, f.repeat)
		}
	}
}

func TestARouteTraceRefusalSaysWhyWithItsOwnStatus(t *testing.T) {
	cases := []struct {
		body string
		err  error
		code int
	}{
		{`nonsense`, nil, http.StatusBadRequest},
		{`{"address":"not an address"}`, nil, http.StatusBadRequest},
		{`{"address":"1.1.1.1"}`, traceroute.ErrBusy, http.StatusConflict},
		{`{"address":"192.168.1.1"}`, traceroute.ErrNotInternet, http.StatusBadRequest},
		{`{"address":"1.1.1.1"}`, traceroute.ErrNotRunning, http.StatusServiceUnavailable},
		{`{"address":"1.1.1.1"}`, errors.New("internet access is off"), http.StatusForbidden},
		{`{"address":"` + strings.Repeat("1", 2000) + `"}`, nil, http.StatusBadRequest},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		NewNetauditRouteTraceHandlers(&fakeTracer{err: c.err}).Start(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)))
		if w.Code != c.code {
			t.Errorf("%.40s %v: %d", c.body, c.err, w.Code)
		}
	}
}

func TestTheKeptRouteTracesAreReadWithWhetherThisSystemCanTrace(t *testing.T) {
	w := httptest.NewRecorder()
	NewNetauditRouteTraceHandlers(&fakeTracer{}).Get(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"supported":`) || !strings.Contains(w.Body.String(), `"traces":[{`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
