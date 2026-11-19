package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

func entry(name, url string) Server { return Server{Name: name, URL: url, Enabled: true} }

func TestPool_StreamableHTTPListsAndCallsTools(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)

	v := byName(p.Views(), "files")
	if v.Status != string(StatusReady) || v.Transport != TransportStreamable || v.ServerName != "fixture" || v.Protocol == "" {
		t.Fatalf("view: %+v", v)
	}
	tools := p.Tools()
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name)
		if tl.Source != "files" || !tl.Ask || !json.Valid(tl.Schema) {
			t.Errorf("tool %+v: source, ask-by-default and a valid schema are expected", tl)
		}
	}
	for _, want := range []string{"mcp__files__echo", "mcp__files__fail", "mcp__files__slow", "mcp__files__big"} {
		if !contains(names, want) {
			t.Errorf("%s is missing from %v", want, names)
		}
	}
	res, err := p.Call(context.Background(), "mcp__files__echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil || res.Text != "echo: hi" || res.IsError {
		t.Fatalf("call: %+v %v", res, err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestPool_ATrustOfAllowMarksTheToolsAsNotAsking(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	s := entry("files", f.URL)
	s.Trust = TrustAllow
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	for _, tl := range p.Tools() {
		if tl.Ask {
			t.Errorf("%s asks although the server is set to allow", tl.Name)
		}
	}
	s.Trust = TrustAsk
	p.Reconcile([]Server{s}, true)
	for _, tl := range p.Tools() {
		if !tl.Ask {
			t.Errorf("%s must ask again at once, without a reconnect", tl.Name)
		}
	}
}

func TestPool_AToolTheOwnerSwitchedOffIsNeitherListedNorCallable(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	s := entry("files", f.URL)
	s.DisabledTools = []string{"echo"}
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	for _, tl := range p.Tools() {
		if tl.Name == "mcp__files__echo" {
			t.Fatal("a switched-off tool must not be listed")
		}
	}
	_, err := p.Call(context.Background(), "mcp__files__echo", json.RawMessage(`{}`))
	if stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Fatalf("a switched-off tool must not run, got %v", err)
	}
	if len(f.Calls()) != 0 {
		t.Error("the server must not have been called")
	}
	v := byName(p.Views(), "files")
	found := false
	for _, tv := range v.Tools {
		if tv.Name == "echo" {
			found = true
			if !tv.Disabled {
				t.Error("the screen must show the tool as switched off")
			}
		}
	}
	if !found {
		t.Error("a switched-off tool stays on the screen so it can be switched on again")
	}
}

func TestPool_AToolErrorIsAResultNotAFailure(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	res, err := p.Call(context.Background(), "mcp__files__fail", nil)
	if err != nil || !res.IsError || res.Text != "it failed" {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestPool_ABigAnswerIsCutBeforeItCrossesBack(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	res, err := p.Call(context.Background(), "mcp__files__big", nil)
	if err != nil || !res.Truncated || len(res.Text) > stacksdk.MaxDynamicResult+len(cutMarker) {
		t.Fatalf("truncated=%v len=%d err=%v", res.Truncated, len(res.Text), err)
	}
}

func TestPool_ASlowToolEndsWithATimeoutNotAHang(t *testing.T) {
	f := startStreamable(t, nil)
	tm := fastTiming()
	tm.Call = 150 * time.Millisecond
	p := NewPool(testHTTP{}, openPlain, nil, tm)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	t.Cleanup(func() { cancel(); p.Stop() })
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	start := time.Now()
	_, err := p.Call(context.Background(), "mcp__files__slow", nil)
	if stacksdk.CodeOf(err) != stacksdk.CodeTimeout {
		t.Fatalf("want a timeout, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("the call took %v", time.Since(start))
	}
}

func TestPool_ABadArgumentsDocumentIsRefusedBeforeTheServerIsCalled(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	_, err := p.Call(context.Background(), "mcp__files__echo", json.RawMessage(`[1,2]`))
	if stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Fatalf("got %v", err)
	}
}

func TestPool_TheOldSSETransportIsTriedWhenTheNewOneIsRefused(t *testing.T) {
	f := startSSE(t)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("old", f.URL)}, true)
	settled(t, p)
	v := byName(p.Views(), "old")
	if v.Status != string(StatusReady) || v.Transport != TransportSSE {
		t.Fatalf("view: %+v", v)
	}
	res, err := p.Call(context.Background(), "mcp__old__echo", json.RawMessage(`{"text":"sse"}`))
	if err != nil || res.Text != "echo: sse" {
		t.Fatalf("call over SSE: %+v %v", res, err)
	}
}

func TestPool_ABearerTokenAndANamedHeaderReachTheServer(t *testing.T) {
	need := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer secret-1" }
	f := startStreamable(t, need)
	p := newTestPool(t, testHTTP{})
	s := entry("files", f.URL)
	s.Auth = Auth{Kind: AuthBearer, Sealed: "sealed:secret-1"}
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	if v := byName(p.Views(), "files"); v.Status != string(StatusReady) {
		t.Fatalf("with the token: %+v", v)
	}
	if !v1(f.Auths(), "Bearer secret-1|") {
		t.Errorf("the bearer header never arrived: %v", f.Auths())
	}

	needKey := func(r *http.Request) bool { return r.Header.Get("X-Api-Key") == "k-2" }
	g := startStreamable(t, needKey)
	h := entry("keyed", g.URL)
	h.Auth = Auth{Kind: AuthHeader, Header: "X-Api-Key", Sealed: "sealed:k-2"}
	p.Reconcile([]Server{h}, true)
	settled(t, p)
	if v := byName(p.Views(), "keyed"); v.Status != string(StatusReady) {
		t.Fatalf("with the header: %+v", v)
	}
}

func v1(list []string, want string) bool { return contains(list, want) }

func TestPool_ARefusedSignInIsSaidInWordsAndNeverShowsTheValue(t *testing.T) {
	f := startStreamable(t, func(*http.Request) bool { return false })
	p := newTestPool(t, testHTTP{})
	s := entry("files", f.URL)
	s.Auth = Auth{Kind: AuthBearer, Sealed: "sealed:wrong-token"}
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	v := byName(p.Views(), "files")
	if v.Status != string(StatusStopped) || v.ProblemCode != string(ProblemSignIn) {
		t.Fatalf("view: %+v", v)
	}
	raw, _ := json.Marshal(p.Views())
	if strings.Contains(string(raw), "wrong-token") || strings.Contains(string(raw), "sealed:") {
		t.Errorf("the screen's view must never carry the sign-in value: %s", raw)
	}
	if !v.HasSecret {
		t.Error("the view says whether a value is set")
	}
}

func TestPool_ASignInThatIsMissingIsReportedAndNothingIsSentUnauthenticated(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	s := entry("files", f.URL)
	s.Auth = Auth{Kind: AuthBearer}
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	if v := byName(p.Views(), "files"); v.ProblemCode != string(ProblemSignInSet) {
		t.Fatalf("view: %+v", v)
	}
	if len(f.Auths()) != 0 {
		t.Errorf("no request may go out when the sign-in is incomplete: %v", f.Auths())
	}
}

func TestPool_ASwitchedOffServerHasNoConnectionAndRemovedOnesAreStopped(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	off := entry("files", f.URL)
	off.Enabled = false
	p.Reconcile([]Server{off}, true)
	if v := byName(p.Views(), "files"); v.Status != StatusOff {
		t.Fatalf("view: %+v", v)
	}
	if len(f.Auths()) != 0 {
		t.Error("a server that is switched off must not be dialled")
	}
	on := off
	on.Enabled = true
	p.Reconcile([]Server{on}, true)
	settled(t, p)
	if len(p.Tools()) == 0 {
		t.Fatal("switched on, the tools appear")
	}
	p.Reconcile(nil, true)
	if len(p.Tools()) != 0 || len(p.Views()) != 0 {
		t.Fatal("a removed server loses its tools and its row")
	}
}

func TestPool_ChangingTheAddressReconnectsAndDropsTheOldTools(t *testing.T) {
	a, b := startStreamable(t, nil), startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", a.URL)}, true)
	settled(t, p)
	p.Reconcile([]Server{entry("files", b.URL)}, true)
	settled(t, p)
	if _, err := p.Call(context.Background(), "mcp__files__echo", json.RawMessage(`{"text":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if len(a.Calls()) != 0 || len(b.Calls()) != 1 {
		t.Errorf("the call must reach the new address: a=%v b=%v", a.Calls(), b.Calls())
	}
}

func TestPool_AServerThatIsDownGivesNoToolsAtOnceAndNoWait(t *testing.T) {
	f := startStreamable(t, nil)
	url := f.URL
	f.http.Close()
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", url)}, true)
	start := time.Now()
	if got := p.Tools(); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Errorf("Tools must answer from the cache, it took %v", time.Since(start))
	}
	settled(t, p)
	if v := byName(p.Views(), "files"); v.Status != string(StatusStopped) || v.ProblemCode != string(ProblemUnreachable) {
		t.Fatalf("view: %+v", v)
	}
	_, err := p.Call(context.Background(), "mcp__files__echo", nil)
	if stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a call to a tool that is not offered now is not found: %v", err)
	}
}

func TestPool_ASessionTheServerForgotIsStartedAgainAndTheCallRetriedOnce(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	if len(p.Tools()) == 0 {
		t.Fatal("first connection")
	}
	// The server forgets every session, as after a restart: the next request on the old session is a 404 and the client must start a new one.
	f.KillSessions()
	res, err := p.Call(context.Background(), "mcp__files__echo", json.RawMessage(`{"text":"again"}`))
	if err != nil || res.Text != "echo: again" {
		t.Fatalf("call after the session ended: %+v %v", res, err)
	}
}

func TestPool_ChangedToolsAreSeenWithoutARestart(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	f.mcp.DeleteTools("echo")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		has := false
		for _, tl := range p.Tools() {
			if tl.Name == "mcp__files__echo" {
				has = true
			}
		}
		if !has {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a tool the server removed must leave the list (the periodic refresh covers a server that never says so)")
}

func TestPool_TestConnectsOnceAndHoldsNothing(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	v := p.Test(context.Background(), entry("files", f.URL))
	if v.Status != string(StatusReady) || len(v.Tools) < 4 || v.Transport != TransportStreamable {
		t.Fatalf("view: %+v", v)
	}
	if len(p.Views()) != 0 || len(p.Tools()) != 0 {
		t.Error("a test must leave no connection behind")
	}
	bad := p.Test(context.Background(), Server{Name: "Bad Name", URL: "x"})
	if bad.Status != string(StatusStopped) || !strings.Contains(bad.Problem, "not valid") {
		t.Errorf("an invalid entry is said to be invalid: %+v", bad)
	}
}

func TestPool_StopWaitsForEveryConnection(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("a", f.URL), entry("b", f.URL)}, true)
	settled(t, p)
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop must return")
	}
	if len(p.Tools()) != 0 {
		t.Error("no tools after Stop")
	}
}

func TestClassify_GivesFixedPhrases(t *testing.T) {
	cases := map[Problem]error{
		ProblemSignIn:      errors.New("request failed with status 401: nope"),
		ProblemForbidden:   errors.New("request failed with status 403: nope"),
		ProblemNotMCP:      errors.New("request failed with status 500: boom"),
		ProblemTLS:         errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
		ProblemTimeout:     context.DeadlineExceeded,
		ProblemGuard:       stacksdk.Errorf(stacksdk.CodeDenied, "x"),
		ProblemUnreachable: errors.New("dial tcp 10.0.0.1:80: connect: connection refused"),
		ProblemVersion:     errVersion,
	}
	for want, err := range cases {
		if got := classify(err); got != want {
			t.Errorf("%v → %q, want %q", err, got, want)
		}
	}
	for _, status := range []string{"400", "404", "405"} {
		if !legacyCandidate(errors.New("request failed with status " + status + ": x")) {
			t.Errorf("%s may mean the old transport", status)
		}
	}
	if legacyCandidate(errors.New("request failed with status 401: x")) || legacyCandidate(errors.New("dial tcp: refused")) {
		t.Error("only 400, 404 and 405 mean the old transport")
	}
}
