package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
)

type recordingRun struct {
	got []stacksdk.PrivilegedRequest
	res stacksdk.PrivilegedResult
	err error
}

func (r *recordingRun) Run(_ context.Context, req stacksdk.PrivilegedRequest) (stacksdk.PrivilegedResult, error) {
	r.got = append(r.got, req)
	return r.res, r.err
}

const helperBody = `{"target":"192.168.50.10","ports":[161,53]}`

func helperServer(minter GrantMinter, run stacksdk.Privileged, on func(string) bool) http.Handler {
	mux := http.NewServeMux()
	for _, rt := range HelperRoutes() {
		mux.HandleFunc(rt.Method+" "+rt.Path, HelperHandler(strings.TrimPrefix(rt.Path, helperPathPrefix), minter, run, on))
	}
	return AuthenticateWith(mux, newFake(), nil)
}

func postHelper(h http.Handler, path, body string, as func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	if as != nil {
		as(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func allOn(string) bool { return true }

// Adding an action to the catalogue without its step-up entry would expose a root-adjacent action to an ordinary admin session.
func TestEveryHelperActionIsAStepUpRoute(t *testing.T) {
	routes := HelperRoutes()
	if len(routes) == 0 {
		t.Fatal("no helper routes: the test would pass over nothing")
	}
	for _, rt := range routes {
		if !slices.Contains(StepUpRoutes(), rt) {
			t.Errorf("%s %s is not in the step-up table", rt.Method, rt.Path)
		}
	}
	if len(routes) != len(catalog.All()) {
		t.Errorf("%d routes for %d actions", len(routes), len(catalog.All()))
	}
}

func TestAHelperRouteNeedsAnElevatedSession(t *testing.T) {
	for _, c := range []struct {
		name string
		as   func(*http.Request)
		want int
	}{
		{"no sign-in", nil, 401},
		{"a plain session", cookie("sess"), 403},
		{"an admin token", bearer("lp_admin"), 403},
		{"an elevated session", cookie("elevated"), 200},
	} {
		m, run := &recordingMinter{}, &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("out")}}
		rec := postHelper(helperServer(m, run, allOn), "/admin/api/helper/scan_udp", helperBody, c.as)
		if rec.Code != c.want {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
		if c.want != 200 && (m.mints != 0 || len(run.got) != 0) {
			t.Errorf("%s: a refused caller must mint nothing and run nothing", c.name)
		}
	}
}

func TestAnActionThatIsOffRunsNothing(t *testing.T) {
	m, run := &recordingMinter{}, &recordingRun{}
	rec := postHelper(helperServer(m, run, func(string) bool { return false }), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if rec.Code != 403 || m.mints != 0 || len(run.got) != 0 {
		t.Errorf("status %d, %d grants, %d runs", rec.Code, m.mints, len(run.got))
	}
	on := func(id string) bool { return id == "os_detect" }
	rec = postHelper(helperServer(m, run, on), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if rec.Code != 403 {
		t.Errorf("only the action that is on may run: %d", rec.Code)
	}
}

func TestAHelperRoutePassesTheGrantTheActionAndTheBodyToThePort(t *testing.T) {
	m, run := &recordingMinter{}, &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("PORT STATE"), Truncated: true}}
	rec := postHelper(helperServer(m, run, allOn), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if rec.Code != 200 || len(run.got) != 1 {
		t.Fatalf("%d %s, %d runs", rec.Code, rec.Body, len(run.got))
	}
	if g := run.got[0]; g.Grant != "grant-1" || g.Action != "scan_udp" || string(g.Params) != helperBody {
		t.Errorf("the port got %+v", g)
	}
	var out HelperResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || string(out.Data) != "PORT STATE" || out.Kind != "bytes" || !out.Truncated {
		t.Errorf("answer %s (%v)", rec.Body, err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
}

func TestABodyTheCatalogueRefusesNeverReachesThePort(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"a public address":    {`{"target":"8.8.8.8","ports":[53]}`, 403},
		"a port off the list": {`{"target":"192.168.50.10","ports":[22]}`, 403},
		"an unknown field":    {`{"target":"192.168.50.10","ports":[53],"script":"vuln"}`, 400},
		"no body":             {``, 400},
	}
	for name, c := range cases {
		m, run := &recordingMinter{}, &recordingRun{}
		rec := postHelper(helperServer(m, run, allOn), "/admin/api/helper/scan_udp", c.body, cookie("elevated"))
		if rec.Code != c.want || m.mints != 0 || len(run.got) != 0 {
			t.Errorf("%s: %d (want %d), %d grants, %d runs", name, rec.Code, c.want, m.mints, len(run.got))
		}
	}
}

func TestAHelperFailureBecomesTheMatchingStatus(t *testing.T) {
	cases := map[stacksdk.Code]int{
		stacksdk.CodeDenied: 403, stacksdk.CodeUnavailable: 503, stacksdk.CodeTimeout: 504, stacksdk.CodeInternal: 500,
	}
	for code, want := range cases {
		m, run := &recordingMinter{}, &recordingRun{err: stacksdk.Errorf(code, "fixed sentence")}
		rec := postHelper(helperServer(m, run, allOn), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
		if rec.Code != want || !strings.Contains(rec.Body.String(), "fixed sentence") {
			t.Errorf("%s: %d %s", code, rec.Code, rec.Body)
		}
	}
}

func TestAFullGrantStoreAnswersUnavailable(t *testing.T) {
	m, run := &recordingMinter{err: context.DeadlineExceeded}, &recordingRun{}
	rec := postHelper(helperServer(m, run, allOn), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if rec.Code != 503 || len(run.got) != 0 {
		t.Errorf("%d, %d runs", rec.Code, len(run.got))
	}
}
