package api

import (
	"context"
	"encoding/json"
	"errors"
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
	return helperServerParsing(minter, run, on, nil)
}

func helperServerParsing(minter GrantMinter, run stacksdk.Privileged, on func(string) bool, parse HelperParser) http.Handler {
	return helperServerKeeping(minter, run, on, parse, nil)
}

func helperServerKeeping(minter GrantMinter, run stacksdk.Privileged, on func(string) bool, parse HelperParser, keep HelperKeeper) http.Handler {
	mux := http.NewServeMux()
	for _, rt := range HelperRoutes() {
		mux.HandleFunc(rt.Method+" "+rt.Path, HelperHandler(strings.TrimPrefix(rt.Path, helperPathPrefix), HelperDeps{Grants: minter, Run: run, On: on, Parse: parse, Keep: keep}))
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

func TestAHelperRouteAnswersTheParsedResultBesideTheRawBytes(t *testing.T) {
	run := &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("<xml/>")}}
	var sawAction string
	parse := func(action string, data []byte) (any, error) {
		sawAction = action
		return map[string]string{"seen": string(data)}, nil
	}
	rec := postHelper(helperServerParsing(&recordingMinter{}, run, allOn, parse), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	var out struct {
		Parsed map[string]string `json:"parsed"`
		Error  string            `json:"parse_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || sawAction != "scan_udp" || out.Parsed["seen"] != "<xml/>" || out.Error != "" {
		t.Errorf("answer %s (%v), parser saw action %q", rec.Body, err, sawAction)
	}
}

func TestAParseFailureKeepsTheRawBytesAndSaysWhy(t *testing.T) {
	run := &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("not xml")}}
	parse := func(string, []byte) (any, error) { return nil, errors.New("boom") }
	rec := postHelper(helperServerParsing(&recordingMinter{}, run, allOn, parse), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	var out struct {
		Data  []byte `json:"data"`
		Error string `json:"parse_error"`
		Parse any    `json:"parsed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); rec.Code != 200 || err != nil || string(out.Data) != "not xml" || out.Error == "" || out.Parse != nil {
		t.Errorf("%d %s (%v)", rec.Code, rec.Body, err)
	}
}

func TestAParsedResultIsKeptWithTheActionAndTheCanonicalParams(t *testing.T) {
	run := &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("<xml/>")}}
	parse := func(string, []byte) (any, error) { return "parsed-value", nil }
	var gotAction string
	var gotParams []byte
	var gotParsed any
	keep := func(action string, params []byte, parsed any) {
		gotAction, gotParams, gotParsed = action, params, parsed
	}
	postHelper(helperServerKeeping(&recordingMinter{}, run, allOn, parse, keep), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if gotAction != "scan_udp" || string(gotParams) != helperBody || gotParsed != "parsed-value" {
		t.Errorf("kept %q %s %v", gotAction, gotParams, gotParsed)
	}
}

func TestNothingIsKeptWhenTheOutputWasCutOrCouldNotBeRead(t *testing.T) {
	for name, res := range map[string]stacksdk.PrivilegedResult{"cut": {Kind: "bytes", Data: []byte("<x"), Truncated: true}, "unreadable": {Kind: "bytes", Data: []byte("x")}} {
		kept := false
		parse := func(string, []byte) (any, error) { return nil, errors.New("bad") }
		keep := func(string, []byte, any) { kept = true }
		postHelper(helperServerKeeping(&recordingMinter{}, &recordingRun{res: res}, allOn, parse, keep), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
		if kept {
			t.Errorf("%s output was kept", name)
		}
	}
}

func TestACutOutputIsNeverParsed(t *testing.T) {
	run := &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("<nmaprun>"), Truncated: true}}
	called := false
	parse := func(string, []byte) (any, error) { called = true; return nil, nil }
	rec := postHelper(helperServerParsing(&recordingMinter{}, run, allOn, parse), "/admin/api/helper/scan_udp", helperBody, cookie("elevated"))
	if called || !strings.Contains(rec.Body.String(), "parse_error") {
		t.Errorf("parser called = %v, answer %s: a cut XML must say it was cut, not be guessed at", called, rec.Body)
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

func helperStatusOf(t *testing.T, src HelperSource) HelperStatus {
	t.Helper()
	rec := httptest.NewRecorder()
	HelperStatusHandler(src).ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/helper", nil))
	var st HelperStatus
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &st) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	return st
}

func source() HelperSource {
	return HelperSource{
		Supported: func() bool { return true }, Installed: func() bool { return true }, Interlock: func() error { return nil },
		Gates:    func() ([]string, bool) { return []string{"scan_udp"}, true },
		Declared: func() []string { return []string{"192.168.50.0/24"} }, On: func(id string) bool { return id == "scan_udp" },
	}
}

func TestHelperStatus_NotInstalledSaysSoAndNamesTheCommand(t *testing.T) {
	src := source()
	src.Installed = func() bool { return false }
	st := helperStatusOf(t, src)
	if st.Installed || st.Ready || st.NotReady == "" || st.InstallCommand != HelperInstallCommand || st.HelpPath == "" {
		t.Errorf("%+v", st)
	}
	if len(st.Actions) != len(catalog.All()) {
		t.Fatalf("every catalogue action is listed, got %d", len(st.Actions))
	}
	for _, a := range st.Actions {
		if a.Installed != nil {
			t.Errorf("%s: installed must be unknown (null) when no helper is there", a.ID)
		}
	}
}

func TestHelperStatus_UnsupportedAndUnsafeHostsAreNotReady(t *testing.T) {
	src := source()
	src.Supported = func() bool { return false }
	if st := helperStatusOf(t, src); st.Supported || st.Ready || !strings.Contains(st.NotReady, "Linux") {
		t.Errorf("%+v", st)
	}
	src = source()
	src.Interlock = func() error { return context.Canceled }
	if st := helperStatusOf(t, src); !st.Installed || st.Ready || !strings.Contains(st.NotReady, "sandbox") {
		t.Errorf("%+v", st)
	}
}

func TestHelperStatus_ReadyReportsTheSwitchesAndTheRanges(t *testing.T) {
	st := helperStatusOf(t, source())
	if !st.Ready || st.NotReady != "" {
		t.Fatalf("%+v", st)
	}
	byID := map[string]HelperAction{}
	for _, a := range st.Actions {
		byID[a.ID] = a
	}
	if !byID["scan_udp"].On || byID["os_detect"].On {
		t.Errorf("the switches are the operator's setting: %+v", byID)
	}
	if in := byID["scan_udp"].Installed; in == nil || !*in {
		t.Errorf("scan_udp is in the install's list: %+v", byID["scan_udp"])
	}
	if in := byID["os_detect"].Installed; in == nil || *in {
		t.Errorf("os_detect is not in the install's list: %+v", byID["os_detect"])
	}
}

func TestHelperStatus_DeclaredRangesAreSortedAndNeverNull(t *testing.T) {
	src := source()
	src.Declared = func() []string { return []string{"192.168.50.0/24", "10.0.0.0/8", "192.168.50.0/24"} }
	if st := helperStatusOf(t, src); !slices.Equal(st.DeclaredRanges, []string{"10.0.0.0/8", "192.168.50.0/24"}) {
		t.Errorf("%v", st.DeclaredRanges)
	}
	src.Declared = func() []string { return nil }
	rec := httptest.NewRecorder()
	HelperStatusHandler(src).ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/helper", nil))
	if !strings.Contains(rec.Body.String(), `"declared_ranges":[]`) {
		t.Errorf("no ranges must encode as a list, not null: %s", rec.Body)
	}
	src.Gates = func() ([]string, bool) { return nil, false }
	for _, a := range helperStatusOf(t, src).Actions {
		if a.Installed != nil {
			t.Errorf("%s: an unreadable install list must read as unknown", a.ID)
		}
	}
}

func TestADefaultsHookCompletesTheBodyBeforeTheGrantIsMinted(t *testing.T) {
	run := &recordingRun{res: stacksdk.PrivilegedResult{Kind: "bytes", Data: []byte("x")}}
	mux := http.NewServeMux()
	deps := HelperDeps{Grants: &recordingMinter{}, Run: run, On: allOn, Defaults: func(action string, body []byte) ([]byte, error) {
		if action != "capture_window" {
			t.Errorf("action %q", action)
		}
		if strings.TrimSpace(string(body)) == "" {
			return []byte(`{"iface":"wlp2s0","seconds":60}`), nil
		}
		return nil, errors.New("pick no interface yourself")
	}}
	mux.HandleFunc("POST /admin/api/helper/capture_window", HelperHandler("capture_window", deps))
	h := AuthenticateWith(mux, newFake(), nil)
	if rec := postHelper(h, "/admin/api/helper/capture_window", "", cookie("elevated")); rec.Code != 200 || len(run.got) != 1 || string(run.got[0].Params) != `{"iface":"wlp2s0","seconds":60}` {
		t.Fatalf("%d %s, params %v", rec.Code, rec.Body, run.got)
	}
	if rec := postHelper(h, "/admin/api/helper/capture_window", `{"iface":"x"}`, cookie("elevated")); rec.Code != 400 || !strings.Contains(rec.Body.String(), "pick no interface") || len(run.got) != 1 {
		t.Fatalf("a refused default must stop the run: %d %s", rec.Code, rec.Body)
	}
}
