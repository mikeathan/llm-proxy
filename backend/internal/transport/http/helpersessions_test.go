package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/platform/privhelper/session"
)

const listenReq = `{"action":"radio_listen","params":{"iface":"wlx1","channel":60,"seconds":20}}`

func sessionMux(t *testing.T, on func(string) bool) (*http.ServeMux, *session.Store) {
	t.Helper()
	store := session.New(session.Options{})
	deps := HelperSessionDeps{Sessions: store, On: on, Stacks: []string{"radio"}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+HelperSessionsPath, HelperSessionsList(deps))
	mux.HandleFunc("POST "+HelperSessionsPath, HelperSessionsBegin(deps))
	mux.HandleFunc("DELETE "+HelperSessionsPath, HelperSessionsEndAll(deps))
	mux.HandleFunc("DELETE "+HelperSessionPath, HelperSessionEnd(deps))
	return mux, store
}

func send(mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestASessionIsApprovedListedAndEnded(t *testing.T) {
	mux, store := sessionMux(t, allOn)
	rec := send(mux, "POST", HelperSessionsPath, `{"stack":"radio","label":"overnight","hours":8,"requests":[`+listenReq+`]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var view session.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || view.ID == "" || view.MaxUses != 8*240 || len(view.Requests) != 1 {
		t.Fatalf("%+v %v", view, err)
	}
	if strings.Contains(rec.Body.String(), "token") {
		t.Error("a session has no token")
	}
	list := send(mux, "GET", HelperSessionsPath, "")
	var got struct {
		Open   []session.View `json:"open"`
		Limits sessionLimits  `json:"limits"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil || len(got.Open) != 1 || got.Limits.MaxHours != 12 {
		t.Fatalf("%s %v", list.Body, err)
	}
	if rec := send(mux, "DELETE", HelperSessionsPath+"/"+view.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("end: %d", rec.Code)
	}
	if rec := send(mux, "DELETE", HelperSessionsPath+"/"+view.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("end again: %d", rec.Code)
	}
	if open, _ := store.Open(); len(open) != 0 {
		t.Error("still open")
	}
}

func TestASessionIsRefusedWhenItAsksForMoreThanMayBeApproved(t *testing.T) {
	mux, _ := sessionMux(t, allOn)
	cases := map[string]struct {
		body string
		want int
	}{
		"another stack":                 {`{"stack":"network","hours":1,"requests":[` + listenReq + `]}`, 400},
		"more than 12 hours":            {`{"stack":"radio","hours":13,"requests":[` + listenReq + `]}`, 400},
		"no time":                       {`{"stack":"radio","requests":[` + listenReq + `]}`, 400},
		"no requests":                   {`{"stack":"radio","hours":1,"requests":[]}`, 400},
		"an action that must not stand": {`{"stack":"radio","hours":1,"requests":[{"action":"scan_udp","params":{"target":"192.168.1.5","ports":[53]}}]}`, 403},
		"an unknown action":             {`{"stack":"radio","hours":1,"requests":[{"action":"nope","params":{}}]}`, 400},
		"a channel outside the list":    {`{"stack":"radio","hours":1,"requests":[{"action":"radio_listen","params":{"iface":"w","channel":14,"seconds":20}}]}`, 403},
		"a window too long":             {`{"stack":"radio","hours":1,"requests":[{"action":"radio_listen","params":{"iface":"w","channel":1,"seconds":31}}]}`, 403},
		"an unknown field":              {`{"stack":"radio","hours":1,"max_hours":99,"requests":[` + listenReq + `]}`, 400},
		"too many uses":                 {`{"stack":"radio","hours":1,"max_uses":5001,"requests":[` + listenReq + `]}`, 400},
		"not json":                      {`nope`, 400},
	}
	for name, c := range cases {
		if rec := send(mux, "POST", HelperSessionsPath, c.body); rec.Code != c.want {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body, c.want)
		}
	}
}

func TestASessionCannotBeStartedForAnActionThatIsOff(t *testing.T) {
	mux, store := sessionMux(t, func(string) bool { return false })
	if rec := send(mux, "POST", HelperSessionsPath, `{"stack":"radio","hours":1,"requests":[`+listenReq+`]}`); rec.Code != http.StatusForbidden {
		t.Errorf("%d", rec.Code)
	}
	if open, _ := store.Open(); len(open) != 0 {
		t.Error("a refused session must not be open")
	}
}

func TestOnlyAFewSessionsMayBeOpen(t *testing.T) {
	mux, _ := sessionMux(t, allOn)
	last := 0
	for i := 0; i <= session.DefaultMaxOpen; i++ {
		last = send(mux, "POST", HelperSessionsPath, `{"stack":"radio","hours":1,"requests":[`+listenReq+`]}`).Code
	}
	if last != http.StatusConflict {
		t.Errorf("%d", last)
	}
	if rec := send(mux, "DELETE", HelperSessionsPath, ""); rec.Code != http.StatusNoContent {
		t.Errorf("end all: %d", rec.Code)
	}
	if got := send(mux, "POST", HelperSessionsPath, `{"stack":"radio","hours":1,"requests":[`+listenReq+`]}`).Code; got != http.StatusCreated {
		t.Errorf("after ending all: %d", got)
	}
}

// Starting a session is a step-up route, and a token principal never gets to a step-up route; listing and ending are not step-up.
func TestStartingASessionNeedsAnElevatedSessionAndEndingDoesNot(t *testing.T) {
	if !hasRoute(StepUpRoutes(), http.MethodPost, HelperSessionsPath) {
		t.Error("POST /admin/api/helper/sessions must be a step-up route")
	}
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		if hasRoute(StepUpRoutes(), m, HelperSessionsPath) {
			t.Errorf("%s need not be step-up: ending only takes authority away", m)
		}
	}
}
