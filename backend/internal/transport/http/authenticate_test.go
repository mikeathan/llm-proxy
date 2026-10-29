package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/auth"
)

type fakeAuth struct {
	state     auth.State
	stateErr  error
	sessions  map[string]auth.Session
	tokens    map[string]auth.Token
	inference bool
}

func (f *fakeAuth) State() (auth.State, error) { return f.state, f.stateErr }
func (f *fakeAuth) Session(id string) (auth.Session, bool) {
	s, ok := f.sessions[id]
	return s, ok
}
func (f *fakeAuth) SessionAlive(id string) bool {
	_, ok := f.sessions[id]
	return ok
}
func (f *fakeAuth) Elevated(s auth.Session) bool { return time.Now().Before(s.ElevatedUntil) }
func (f *fakeAuth) Token(plain string) (auth.Token, bool) {
	t, ok := f.tokens[plain]
	return t, ok
}
func (f *fakeAuth) InferenceRequired() bool { return f.inference }

func newFake() *fakeAuth {
	return &fakeAuth{
		state:    auth.State{Required: true},
		sessions: map[string]auth.Session{"sess": {}, "elevated": {ElevatedUntil: time.Now().Add(time.Minute)}},
		tokens: map[string]auth.Token{
			"lp_admin": {ID: "a", Scope: auth.ScopeAdmin}, "lp_inference": {ID: "i", Scope: auth.ScopeInference},
		},
	}
}

func authed(a Authenticator) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return Authenticate(ok, a)
}

func req(h http.Handler, method, path string, mod func(*http.Request)) int {
	r := httptest.NewRequest(method, path, nil)
	if mod != nil {
		mod(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func cookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: SessionCookie, Value: v}) }
}
func bearer(v string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
}

func TestAuthenticate_NothingIsRequiredUntilAPassphraseExists(t *testing.T) {
	f := newFake()
	f.state = auth.State{}
	h := authed(f)
	for _, p := range []string{"/admin/api/state", "/admin/api/system/wipeout", "/v1/models"} {
		if code := req(h, "POST", p, nil); code != http.StatusOK {
			t.Errorf("%s while unset = %d, want 200 (the upgrade state is open)", p, code)
		}
	}
}

func TestAuthenticate_AdminAPIRequiresACredentialOnceSetUp(t *testing.T) {
	h := authed(newFake())
	cases := []struct {
		name string
		mod  func(*http.Request)
		want int
	}{
		{"nothing", nil, http.StatusUnauthorized},
		{"a session cookie", cookie("sess"), http.StatusOK},
		{"a made-up cookie", cookie("forged"), http.StatusUnauthorized},
		{"an admin token", bearer("lp_admin"), http.StatusOK},
		{"an inference token", bearer("lp_inference"), http.StatusUnauthorized},
		{"a made-up token", bearer("lp_forged"), http.StatusUnauthorized},
	}
	for _, c := range cases {
		if code := req(h, "GET", "/admin/api/state", c.mod); code != c.want {
			t.Errorf("%s = %d, want %d", c.name, code, c.want)
		}
	}
}

func TestAuthenticate_OnlyTheSignInRoutesAndTheShellAreOpen(t *testing.T) {
	h := authed(newFake())
	open := []struct{ method, path string }{
		{"GET", "/admin/"}, {"GET", "/admin/assets/index.js"}, {"GET", "/admin/api/auth/status"},
		{"POST", "/admin/api/auth/login"}, {"POST", "/admin/api/auth/setup"}, {"POST", "/admin/api/auth/logout"},
		{"POST", "/api/v1/webhooks/telegram"}, // its own secret
	}
	for _, c := range open {
		if code := req(h, c.method, c.path, nil); code != http.StatusOK {
			t.Errorf("%s %s = %d, want open", c.method, c.path, code)
		}
	}
	closed := []struct{ method, path string }{
		{"GET", "/admin/api/auth/sessions"}, {"GET", "/admin/api/auth/tokens"}, {"POST", "/admin/api/auth/reauth"},
		{"POST", "/admin/api/auth/status"}, {"GET", "/admin/api/auth/login"},
	}
	for _, c := range closed {
		if code := req(h, c.method, c.path, nil); code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, code)
		}
	}
}

func TestAuthenticate_AnUnreadableStoreRefuses(t *testing.T) {
	f := newFake()
	f.stateErr = http.ErrAbortHandler
	h := authed(f)
	if code := req(h, "GET", "/admin/api/state", cookie("sess")); code != http.StatusServiceUnavailable {
		t.Errorf("= %d, want 503: a damaged store must never read as open", code)
	}
	if code := req(h, "GET", "/admin/", nil); code != http.StatusOK {
		t.Errorf("the shell must still load so the page can say what is wrong: %d", code)
	}
}

func TestAuthenticate_InferenceKeyOnlyOnceOneExists(t *testing.T) {
	f := newFake()
	h := authed(f)
	if code := req(h, "POST", "/v1/chat/completions", nil); code != http.StatusUnauthorized && f.inference {
		t.Fatal("setup")
	}
	if code := req(h, "POST", "/v1/chat/completions", nil); code != http.StatusOK {
		t.Errorf("no inference key exists, so /v1 stays open: %d", code)
	}
	f.inference = true
	cases := []struct {
		name string
		mod  func(*http.Request)
		want int
	}{
		{"no key", nil, http.StatusUnauthorized},
		{"an inference token", bearer("lp_inference"), http.StatusOK},
		{"an admin token", bearer("lp_admin"), http.StatusOK},
		{"a made-up key", bearer("lp_forged"), http.StatusUnauthorized},
		{"a session cookie is not a model key", cookie("sess"), http.StatusUnauthorized},
	}
	for _, c := range cases {
		if code := req(h, "POST", "/v1/chat/completions", c.mod); code != c.want {
			t.Errorf("%s = %d, want %d", c.name, code, c.want)
		}
	}
	if code := req(h, "DELETE", "/v1/queue/abc", nil); code != http.StatusUnauthorized {
		t.Errorf("the queue cancel is part of /v1: %d", code)
	}
}

func TestAuthenticate_InferenceRequirementIsIndependentOfAdminSignIn(t *testing.T) {
	f := newFake()
	f.state = auth.State{} // no passphrase yet
	f.inference = true
	if code := req(authed(f), "GET", "/v1/models", nil); code != http.StatusUnauthorized {
		t.Errorf("an inference key was made, so /v1 needs it even before a passphrase: %d", code)
	}
}

func TestAuthenticate_StepUpRoutes(t *testing.T) {
	h := authed(newFake())
	for _, route := range stepUpProbes {
		if code := req(h, route.method, route.path, cookie("sess")); code != http.StatusForbidden {
			t.Errorf("%s %s with a plain session = %d, want 403 reauth_required", route.method, route.path, code)
		}
		if code := req(h, route.method, route.path, cookie("elevated")); code != http.StatusOK {
			t.Errorf("%s %s with an elevated session = %d, want 200", route.method, route.path, code)
		}
		if code := req(h, route.method, route.path, bearer("lp_admin")); code != http.StatusForbidden {
			t.Errorf("%s %s with an admin token = %d, want 403: a token cannot confirm a passphrase", route.method, route.path, code)
		}
	}
	// The same routes are not gated while nothing is set up, and ordinary routes are never gated.
	if code := req(h, "PUT", "/admin/api/dispatcher/workspaces/ws/files/a.md", cookie("sess")); code != http.StatusOK {
		t.Errorf("an ordinary write = %d, want 200", code)
	}
	if code := req(h, "GET", "/admin/api/secrets/keys", cookie("sess")); code != http.StatusOK {
		t.Errorf("reading the masked key list is not a step-up: %d", code)
	}
}

type probe struct{ method, path string }

var stepUpProbes = []probe{
	{"POST", "/admin/api/system/wipeout"}, {"POST", "/admin/api/system/factory-reset"}, {"POST", "/admin/api/system/clear-runtime-data"},
	{"PUT", "/admin/api/host"}, {"PUT", "/admin/api/secrets/keys"}, {"DELETE", "/admin/api/secrets/keys"}, {"PUT", "/admin/api/secrets/tools"},
	{"POST", "/admin/api/mcp"}, {"PUT", "/admin/api/mcp"}, {"DELETE", "/admin/api/mcp"}, {"POST", "/admin/api/auth/tokens"},
}

func TestAuthenticate_StepUpNotEnforcedWhileOpen(t *testing.T) {
	f := newFake()
	f.state = auth.State{}
	if code := req(authed(f), "POST", "/admin/api/system/wipeout", nil); code != http.StatusOK {
		t.Errorf("= %d", code)
	}
}

func TestAuthenticate_ThePrincipalReachesTheHandler(t *testing.T) {
	var got Principal
	var ok bool
	h := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, ok = PrincipalFrom(r.Context()) }), newFake())
	req(h, "GET", "/admin/api/state", cookie("sess"))
	if !ok || got.Kind != PrincipalSession || got.SessionID != "sess" {
		t.Errorf("session principal = %+v %v", got, ok)
	}
	req(h, "GET", "/admin/api/state", bearer("lp_admin"))
	if !ok || got.Kind != PrincipalToken || got.Token.ID != "a" {
		t.Errorf("token principal = %+v %v", got, ok)
	}
}

func TestStepUpRoutesAreAllRegisteredAsWrites(t *testing.T) {
	for _, r := range StepUpRoutes() {
		if r.Method == http.MethodGet {
			t.Errorf("%s %s: a step-up route is a write", r.Method, r.Path)
		}
	}
}

func TestStepUpGranted_TellsAHandlerWhetherThePassphraseWasRecentlyEntered(t *testing.T) {
	var granted bool
	var seen bool
	h := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { granted, seen = StepUpGranted(r.Context()), true }), newFake())
	check := func(name string, mod func(*http.Request), want bool) {
		granted, seen = false, false
		req(h, "PUT", "/admin/api/config", mod)
		if !seen || granted != want {
			t.Errorf("%s: seen %v granted %v, want granted %v", name, seen, granted, want)
		}
	}
	check("an elevated session", cookie("elevated"), true)
	check("a plain session", cookie("sess"), false)
	check("an admin token", bearer("lp_admin"), false)

	f := newFake()
	f.state = auth.State{}
	h = Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { granted, seen = StepUpGranted(r.Context()), true }), f)
	check("sign-in not set up", nil, true)
}

func TestStepUpGranted_WithoutTheMiddlewareThereIsNothingToAskOf(t *testing.T) {
	if !StepUpGranted(httptest.NewRequest("PUT", "/", nil).Context()) {
		t.Error("a handler run without the middleware (a unit test) must not be refused: the middleware is what decides")
	}
}

func TestRefuseStepUp_SaysWhatToDo(t *testing.T) {
	for _, c := range []struct {
		name string
		mod  func(*http.Request)
		want string
	}{
		{"a session", cookie("sess"), "reauth_required"},
		{"a token", bearer("lp_admin"), "needs the passphrase"},
	} {
		var body string
		var code int
		h := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := httptest.NewRecorder()
			RefuseStepUp(rec, r)
			code, body = rec.Code, rec.Body.String()
		}), newFake())
		req(h, "PUT", "/admin/api/config", c.mod)
		if code != http.StatusForbidden || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %q, want 403 containing %q", c.name, code, body, c.want)
		}
	}
}

// handlerSeeing runs the middleware around a handler that records what StillAuthorized says for its request.
func handlerSeeing(a Authenticator, got *[]bool) http.Handler {
	return Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = append(*got, StillAuthorized(r.Context()))
	}), a)
}

func serve(h http.Handler, mod func(*http.Request)) {
	r := httptest.NewRequest(http.MethodGet, "/admin/api/dispatcher/workspaces/w/live", nil)
	mod(r)
	h.ServeHTTP(httptest.NewRecorder(), r)
}

// An open stream asks again whether the credential it started with still works: a session that was signed out or
// revoked, a revoked token, and a store that cannot be read all end it.
func TestStillAuthorized_FollowsTheCredentialTheRequestStartedWith(t *testing.T) {
	f := newFake()
	var seen []bool
	h := handlerSeeing(f, &seen)

	serve(h, cookie("sess"))
	serve(h, func(r *http.Request) { r.Header.Set("Authorization", "Bearer lp_admin") })
	if len(seen) != 2 || !seen[0] || !seen[1] {
		t.Fatalf("a valid session and token must read as authorized: %v", seen)
	}

	var still func() bool
	h2 := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		still = func() bool { return StillAuthorized(ctx) }
	}), f)
	serve(h2, cookie("sess"))
	delete(f.sessions, "sess")
	if still() {
		t.Error("a revoked session still read as authorized")
	}

	serve(h2, func(r *http.Request) { r.Header.Set("Authorization", "Bearer lp_admin") })
	delete(f.tokens, "lp_admin")
	if still() {
		t.Error("a revoked token still read as authorized")
	}

	f.sessions["sess"] = auth.Session{}
	serve(h2, cookie("sess"))
	f.stateErr = errors.New("unreadable")
	if still() {
		t.Error("an unreadable sign-in store read as authorized")
	}
}

// Before a passphrase exists nothing is signed in and nothing is revoked; and a request that never went through the
// middleware (a test, a handler used on its own) has nothing to re-check.
func TestStillAuthorized_OpenWhenNoSignInIsRequired(t *testing.T) {
	if !StillAuthorized(context.Background()) {
		t.Error("a context the middleware never saw must read as authorized")
	}
	f := newFake()
	f.state.Required = false
	var seen []bool
	serve(handlerSeeing(f, &seen), func(*http.Request) {})
	if len(seen) != 1 || !seen[0] {
		t.Errorf("open API: %v", seen)
	}
}
