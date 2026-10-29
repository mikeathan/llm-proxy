package handlers

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/platform/auth"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

const testPass = "correct horse battery"

func newAuthHandlers(t *testing.T) (*AuthHandlers, *auth.Service) {
	t.Helper()
	svc := auth.NewService(auth.OpenStore(filepath.Join(t.TempDir(), "auth.json")), auth.NewHasher(auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}), auth.DefaultConfig())
	return NewAuthHandlers(svc), svc
}

func call(h http.HandlerFunc, method, target, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.7:5555"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Gecko/20100101 Firefox/130.0")
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == api.SessionCookie {
			return c
		}
	}
	return nil
}

func withCookie(c *http.Cookie) func(*http.Request) { return func(r *http.Request) { r.AddCookie(c) } }

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m
}

// setUpAndSignIn runs the first-run path through the handlers and returns the session cookie.
func setUpAndSignIn(t *testing.T, h *AuthHandlers, svc *auth.Service) *http.Cookie {
	t.Helper()
	code, err := svc.IssueSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	rec := call(h.Setup, "POST", "/", `{"code":"`+code+`","passphrase":"`+testPass+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("setup does not sign the operator in")
	}
	return c
}

func TestAuthStatus_NeverSaysMoreThanItMust(t *testing.T) {
	h, svc := newAuthHandlers(t)
	m := decode(t, call(h.Status, "GET", "/", "", nil))
	if m["set_up"] != false || m["setup_open"] != false || m["authenticated"] != false {
		t.Errorf("fresh: %v", m)
	}
	if len(m) > 5 {
		t.Errorf("the public status says too much: %v", m)
	}
	c := setUpAndSignIn(t, h, svc)
	m = decode(t, call(h.Status, "GET", "/", "", nil))
	if m["set_up"] != true || m["authenticated"] != false {
		t.Errorf("signed out: %v", m)
	}
	m = decode(t, call(h.Status, "GET", "/", "", withCookie(c)))
	if m["authenticated"] != true {
		t.Errorf("signed in: %v", m)
	}
}

func TestAuthLogin_CookieFlagsAndFailures(t *testing.T) {
	h, svc := newAuthHandlers(t)
	setUpAndSignIn(t, h, svc)

	rec := call(h.Login, "POST", "/", `{"passphrase":"`+testPass+`"}`, nil)
	c := sessionCookie(rec)
	if rec.Code != http.StatusOK || c == nil {
		t.Fatalf("login = %d", rec.Code)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/admin" || c.Secure || c.MaxAge <= 0 {
		t.Errorf("cookie flags: %+v (plain HTTP must not set Secure, or the cookie would never be sent)", c)
	}
	if strings.Contains(rec.Body.String(), c.Value) {
		t.Error("the session id is in the response body as well as the cookie")
	}
	tls := call(h.Login, "POST", "/", `{"passphrase":"`+testPass+`"}`, func(r *http.Request) { r.TLS = &tls.ConnectionState{} })
	if c := sessionCookie(tls); c == nil || !c.Secure {
		t.Errorf("over HTTPS the cookie must be Secure: %+v", c)
	}

	bad := call(h.Login, "POST", "/", `{"passphrase":"not the passphrase"}`, nil)
	if bad.Code != http.StatusUnauthorized || sessionCookie(bad) != nil {
		t.Errorf("wrong passphrase = %d", bad.Code)
	}
	if call(h.Login, "POST", "/", `{}`, nil).Code != http.StatusBadRequest {
		t.Error("an empty body must be a 400")
	}
}

// Behind a TLS front end the server sees plain HTTP: the cookie is Secure and the status says "encrypted" only when the
// proxy is one the operator listed, because anyone can send X-Forwarded-Proto.
func TestAuthLogin_FollowsATrustedProxyForSecureAndEncrypted(t *testing.T) {
	h, svc := newAuthHandlers(t)
	setUpAndSignIn(t, h, svc)
	h = h.WithTrustedProxies(func() []string { return []string{"10.0.0.0/24"} })
	login := `{"passphrase":"` + testPass + `"}`
	forwarded := func(peer string) func(*http.Request) {
		return func(r *http.Request) { r.RemoteAddr = peer; r.Header.Set("X-Forwarded-Proto", "https") }
	}

	if c := sessionCookie(call(h.Login, "POST", "/", login, forwarded("10.0.0.7:5555"))); c == nil || !c.Secure {
		t.Errorf("behind a listed proxy the cookie must be Secure: %+v", c)
	}
	if c := sessionCookie(call(h.Login, "POST", "/", login, forwarded("203.0.113.9:5555"))); c == nil || c.Secure {
		t.Errorf("a header from a stranger must not make the cookie Secure: %+v", c)
	}
	if m := decode(t, call(h.Status, "GET", "/", "", forwarded("10.0.0.7:5555"))); m["encrypted"] != true {
		t.Errorf("status behind a listed proxy: %v", m)
	}
	if m := decode(t, call(h.Status, "GET", "/", "", forwarded("203.0.113.9:5555"))); m["encrypted"] != false {
		t.Errorf("status for a forged header: %v", m)
	}
}

// Behind a trusted proxy a wrong passphrase is recorded against the person's own address, not the proxy's: otherwise one
// person's failures would lock everyone out, and the sign-in log could not say who tried.
func TestAuthLogin_RecordsTheForwardedClientBehindATrustedProxy(t *testing.T) {
	h, svc := newAuthHandlers(t)
	setUpAndSignIn(t, h, svc)
	h = h.WithTrustedProxies(func() []string { return []string{"10.0.0.0/24"} })
	bad := `{"passphrase":"not the passphrase"}`

	call(h.Login, "POST", "/", bad, func(r *http.Request) { r.RemoteAddr = "10.0.0.7:5555"; r.Header.Set("X-Forwarded-For", "192.168.1.20") })
	call(h.Login, "POST", "/", bad, func(r *http.Request) { r.RemoteAddr = "203.0.113.9:5555"; r.Header.Set("X-Forwarded-For", "6.6.6.6") })

	var seen []string
	for _, e := range svc.Events() {
		if e.Kind == "login_failed" {
			seen = append(seen, e.Address)
		}
	}
	if len(seen) != 2 || seen[0] != "192.168.1.20" || seen[1] != "203.0.113.9" {
		t.Errorf("failed sign-ins were recorded against %v, want the forwarded client then the stranger's own address", seen)
	}
}

func TestAuthLogin_LockoutIs429WithRetryAfter(t *testing.T) {
	h, svc := newAuthHandlers(t)
	setUpAndSignIn(t, h, svc)
	var rec *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		rec = call(h.Login, "POST", "/", `{"passphrase":"not the passphrase"}`, nil)
	}
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("after repeated failures = %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestAuthSetup_Refusals(t *testing.T) {
	h, svc := newAuthHandlers(t)
	if rec := call(h.Setup, "POST", "/", `{"code":"ABCD-2345","passphrase":"`+testPass+`"}`, nil); rec.Code != http.StatusConflict && rec.Code != http.StatusForbidden {
		t.Errorf("no code was issued: %d", rec.Code)
	}
	code, _ := svc.IssueSetupCode()
	if rec := call(h.Setup, "POST", "/", `{"code":"WRONG-CODE","passphrase":"`+testPass+`"}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("wrong code = %d", rec.Code)
	}
	rec := call(h.Setup, "POST", "/", `{"code":"`+code+`","passphrase":"short"}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at least") {
		t.Errorf("weak passphrase = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthLogout_ClearsTheCookieAndTheSession(t *testing.T) {
	h, svc := newAuthHandlers(t)
	c := setUpAndSignIn(t, h, svc)
	rec := call(h.Logout, "POST", "/", "", withCookie(c))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if got := sessionCookie(rec); got == nil || got.MaxAge >= 0 {
		t.Errorf("the cookie must be cleared: %+v", got)
	}
	if _, ok := svc.Session(c.Value); ok {
		t.Error("the session survived logout")
	}
	if call(h.Logout, "POST", "/", "", nil).Code != http.StatusNoContent {
		t.Error("logging out when not signed in is fine")
	}
}

func TestAuthReauth_RotatesTheCookie(t *testing.T) {
	h, svc := newAuthHandlers(t)
	c := setUpAndSignIn(t, h, svc)
	if rec := call(h.Reauth, "POST", "/", `{"passphrase":"nope nope nope nope"}`, withCookie(c)); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong = %d", rec.Code)
	}
	rec := call(h.Reauth, "POST", "/", `{"passphrase":"`+testPass+`"}`, withCookie(c))
	fresh := sessionCookie(rec)
	if rec.Code != http.StatusOK || fresh == nil || fresh.Value == c.Value {
		t.Fatalf("reauth = %d, cookie %+v", rec.Code, fresh)
	}
	if call(h.Reauth, "POST", "/", `{"passphrase":"`+testPass+`"}`, nil).Code != http.StatusUnauthorized {
		t.Error("reauth with no session must be 401")
	}
}

func TestAuthSessionsAndTokens(t *testing.T) {
	h, svc := newAuthHandlers(t)
	c := setUpAndSignIn(t, h, svc)
	second := sessionCookie(call(h.Login, "POST", "/", `{"passphrase":"`+testPass+`"}`, nil))

	var list []map[string]any
	rec := call(h.Sessions, "GET", "/", "", withCookie(c))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("sessions = %s", rec.Body.String())
	}
	var otherID string
	for _, s := range list {
		if s["current"] == false {
			otherID = s["id"].(string)
		}
		if s["browser"] == nil || !strings.Contains(s["browser"].(string), "Firefox") {
			t.Errorf("the browser should be named: %v", s)
		}
	}
	if rec := call(h.RevokeSession, "DELETE", "/?id="+otherID, "", withCookie(c)); rec.Code != http.StatusNoContent {
		t.Errorf("revoke = %d", rec.Code)
	}
	if _, ok := svc.Session(second.Value); ok {
		t.Error("the revoked session still works")
	}
	if rec := call(h.RevokeSession, "DELETE", "/?id=nope", "", withCookie(c)); rec.Code != http.StatusNotFound {
		t.Errorf("revoke unknown = %d", rec.Code)
	}

	rec = call(h.CreateToken, "POST", "/", `{"name":"eval scripts","scope":"admin"}`, withCookie(c))
	m := decode(t, rec)
	tok, _ := m["token"].(string)
	if rec.Code != http.StatusOK || !strings.HasPrefix(tok, auth.TokenPrefix) {
		t.Fatalf("create token = %d %s", rec.Code, rec.Body.String())
	}
	listed := call(h.Tokens, "GET", "/", "", withCookie(c)).Body.String()
	if strings.Contains(listed, tok) || strings.Contains(listed, "hash") || !strings.Contains(listed, "eval scripts") {
		t.Errorf("the token list: %s", listed)
	}
	if call(h.CreateToken, "POST", "/", `{"name":"","scope":"admin"}`, withCookie(c)).Code != http.StatusBadRequest {
		t.Error("an empty name must be a 400")
	}
	id := m["id"].(string)
	if rec := call(h.RevokeToken, "DELETE", "/?id="+id, "", withCookie(c)); rec.Code != http.StatusNoContent {
		t.Errorf("revoke token = %d", rec.Code)
	}
}

func TestAuthChangePassphrase(t *testing.T) {
	h, svc := newAuthHandlers(t)
	c := setUpAndSignIn(t, h, svc)
	if rec := call(h.ChangePassphrase, "POST", "/", `{"old":"nope nope nope nope","new":"a brand new passphrase"}`, withCookie(c)); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong old = %d", rec.Code)
	}
	if rec := call(h.ChangePassphrase, "POST", "/", `{"old":"`+testPass+`","new":"short"}`, withCookie(c)); rec.Code != http.StatusBadRequest {
		t.Errorf("weak new = %d", rec.Code)
	}
	if rec := call(h.ChangePassphrase, "POST", "/", `{"old":"`+testPass+`","new":"a brand new passphrase"}`, withCookie(c)); rec.Code != http.StatusNoContent {
		t.Errorf("change = %d", rec.Code)
	}
}

func TestBrowserName(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Macintosh) Gecko/20100101 Firefox/130.0":                                  "Firefox on Mac",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120 Safari/537.36 Edg/120":     "Edge on Windows",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36":           "Chrome on Linux",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit/605 Version/17 Mobile Safari/604": "Safari on iPhone",
		"curl/8.4.0": "curl", "": "Unknown browser",
	}
	for ua, want := range cases {
		if got := browserName(ua); got != want {
			t.Errorf("%q = %q, want %q", ua, got, want)
		}
	}
}

// session length: the stored settings, as the page sees them, and a save that is checked before it is stored.
type memoryLifetimes struct {
	cur   models.AuthSettings
	saved int
	err   error
}

func (m *memoryLifetimes) get() models.AuthSettings { return m.cur }
func (m *memoryLifetimes) set(a models.AuthSettings) error {
	if m.err != nil {
		return m.err
	}
	m.cur, m.saved = a, m.saved+1
	return nil
}

func TestSessionLength_ShowsTheEffectiveValuesAndTheBounds(t *testing.T) {
	h, _ := newAuthHandlers(t)
	mem := &memoryLifetimes{}
	h = h.WithSessionLength(mem.get, mem.set)

	m := decode(t, call(h.SessionLength, "GET", "/", "", nil))
	if m["idle_hours"] != float64(auth.DefaultIdleHours) || m["absolute_days"] != float64(auth.DefaultAbsoluteDays) {
		t.Errorf("unset settings must show the defaults: %v", m)
	}
	if m["max_idle_hours"] != float64(auth.MaxIdleHours) || m["max_absolute_days"] != float64(auth.MaxAbsoluteDays) {
		t.Errorf("bounds: %v", m)
	}
	mem.cur = models.AuthSettings{IdleHours: 2, AbsoluteDays: 3}
	if m := decode(t, call(h.SessionLength, "GET", "/", "", nil)); m["idle_hours"] != float64(2) || m["absolute_days"] != float64(3) {
		t.Errorf("set values: %v", m)
	}
}

func TestSessionLength_SavesOnlyWhatIsValid(t *testing.T) {
	h, _ := newAuthHandlers(t)
	mem := &memoryLifetimes{}
	h = h.WithSessionLength(mem.get, mem.set)
	put := func(body string) *httptest.ResponseRecorder { return call(h.SetSessionLength, "PUT", "/", body, nil) }

	if rec := put(`{"idle_hours":12,"absolute_days":30}`); rec.Code != http.StatusOK || mem.cur != (models.AuthSettings{IdleHours: 12, AbsoluteDays: 30}) {
		t.Fatalf("valid save = %d %s, stored %+v", rec.Code, rec.Body.String(), mem.cur)
	}
	for _, body := range []string{`{"idle_hours":0,"absolute_days":30}`, `{"idle_hours":12,"absolute_days":9999}`, `{"idle_hours":100,"absolute_days":2}`, `{}`, `{bad`} {
		before := mem.saved
		if rec := put(body); rec.Code != http.StatusBadRequest || mem.saved != before {
			t.Errorf("%s: status %d, saves %d -> %d; a refused value must not be stored", body, rec.Code, before, mem.saved)
		}
	}
	mem.err = errors.New("disk full")
	if rec := put(`{"idle_hours":12,"absolute_days":30}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("a failed write = %d, want 500", rec.Code)
	}
}
