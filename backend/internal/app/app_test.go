package app_test

import (
	"context"
	"encoding/json"
	"llm-proxy/internal/platform/auth"
	api "llm-proxy/internal/transport/http"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func TestAppBoots(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	if a == nil {
		t.Fatal("app not initialized")
	}
}

func TestRoutesExist(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	tests := []string{
		"/admin/",
		"/admin/api/state",
		"/v1/chat/completions",
		"/admin/api/conversation/message",
		// The caller's out-of-band cancel for a held model request. A GET here
		// must still resolve (405), which is what proves the route exists.
		"/v1/queue/inbound:probe",
	}

	for _, path := range tests {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = testHost // the admin interface refuses names it does not know; example.com would be a 421, not a route check
		rec := httptest.NewRecorder()

		a.Handler().ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s", path)
		}
	}
}

// testHost is a Host the admin interface accepts (an IP literal).
const testHost = "127.0.0.1:4001"

func TestMethodEnforcement(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	// Sign-in is compulsory, so the router is only reached by someone signed in.
	cookie := signInThroughTheCLI(t, a, dataMgr)

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
	req.Host = testHost
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	a.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// signInThroughTheCLI sets the passphrase the way an operator does (a setup code made by a second process on the same file,
// entered on the setup route) and returns the session cookie it was given.
func signInThroughTheCLI(t *testing.T, a *app.App, dataMgr *storage.DataManager) *http.Cookie {
	t.Helper()
	cli := auth.NewService(auth.OpenStore(app.AuthFilePath(dataMgr.RootDir())), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig())
	code, err := cli.IssueSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/admin/api/auth/setup", strings.NewReader(`{"code":"`+code+`","passphrase":"correct horse battery"}`))
	req.Host = testHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == api.SessionCookie {
			return c
		}
	}
	t.Fatal("no session cookie after setup")
	return nil
}

func minimalTestDataManager(t *testing.T) *storage.DataManager {
	dir := t.TempDir()

	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:            ":0",
			ModelHost:       "http://localhost",
			IdleTimeoutSecs: 10,
		},
		Models: []models.ModelConfig{},
	}

	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)

	mgr, err := storage.NewDataManager(seededPaths(t, dir))
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}

	_ = mgr.LoadAll()
	return mgr
}

// The guard is wired around the whole router: a foreign name and a cross-site request never reach a handler, while the
// UI's own requests and the model proxy are untouched.
func TestHandlerIsGuarded(t *testing.T) {
	utils.SetRequiredEnv(t)
	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	serve := func(method, path, host, site string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Host = host
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := serve("GET", "/admin/api/state", "evil.example", ""); code != http.StatusMisdirectedRequest {
		t.Errorf("a foreign Host = %d, want 421", code)
	}
	if code := serve("POST", "/admin/api/system/wipeout", testHost, "cross-site"); code != http.StatusForbidden {
		t.Errorf("a cross-site POST = %d, want 403", code)
	}
	if code := serve("GET", "/admin/api/providers/test", testHost, "cross-site"); code != http.StatusForbidden {
		t.Errorf("a cross-site GET = %d, want 403", code)
	}
	if code := serve("GET", "/v1/models", "vertex.lan", "cross-site"); code == http.StatusMisdirectedRequest || code == http.StatusForbidden {
		t.Errorf("/v1 must not be guarded by the browser rules, got %d", code)
	}
}

// The whole path, through the real handler: sign-in is off until the operator sets it up with a code the command line
// made (a second process on the same file), then every admin route needs the cookie, and a destructive one needs the
// passphrase again. The model proxy and the UI shell stay open.
func TestSignInEndToEnd(t *testing.T) {
	utils.SetRequiredEnv(t)
	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	send := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = testHost
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec
	}

	// Sign-in is compulsory: before a passphrase exists the admin API refuses everything but the sign-in routes.
	if rec := send("GET", "/admin/api/version", "", nil); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "setup_required") {
		t.Fatalf("before setup the admin API must answer 401 setup_required: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send("GET", "/admin/api/auth/status", "", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"set_up":false`) {
		t.Fatalf("status must stay open before setup: %d %s", rec.Code, rec.Body.String())
	}

	cli := auth.NewService(auth.OpenStore(app.AuthFilePath(dataMgr.RootDir())), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig())
	code, err := cli.IssueSetupCode()
	if err != nil {
		t.Fatal(err)
	}
	rec := send("POST", "/admin/api/auth/setup", `{"code":"`+code+`","passphrase":"correct horse battery"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == api.SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie after setup")
	}

	if rec := send("GET", "/admin/api/version", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("after setup, no credential = %d, want 401", rec.Code)
	}
	if rec := send("GET", "/admin/api/version", "", cookie); rec.Code != http.StatusOK {
		t.Errorf("with the cookie = %d, want 200", rec.Code)
	}
	if rec := send("POST", "/admin/api/system/wipeout", "", cookie); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "reauth_required") {
		t.Errorf("wipeout with a plain session = %d %s, want 403 reauth_required", rec.Code, rec.Body.String())
	}
	if rec := send("GET", "/admin/", "", nil); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("the UI shell must stay open: %d", rec.Code)
	}
	if rec := send("GET", "/v1/models", "", nil); rec.Code == http.StatusUnauthorized {
		t.Errorf("/v1 stays open until an inference key exists: %d", rec.Code)
	}
	if rec := send("GET", "/admin/api/auth/status", "", nil); !strings.Contains(rec.Body.String(), `"set_up":true`) {
		t.Errorf("status = %s", rec.Body.String())
	}

	// `auth reset` (a second process again) closes the API: the old cookie is dead and the API asks to be set up again.
	if err := cli.Reset(); err != nil {
		t.Fatal(err)
	}
	if rec := send("GET", "/admin/api/version", "", cookie); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "setup_required") {
		t.Errorf("after a reset the old session = %d %s, want 401 setup_required", rec.Code, rec.Body.String())
	}
}

// A settings save is ordinary until it changes a program to run (the model server's arguments, environment or path, or the
// GPU program): that is command execution, so it needs the passphrase again, and a script token can never do it.
func TestSignIn_ChangingWhatRunsNeedsThePassphraseAgain(t *testing.T) {
	utils.SetRequiredEnv(t)
	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)
	send := func(method, path, body string, cookie *http.Cookie, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = testHost
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec
	}
	sessionOf := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == api.SessionCookie {
				return c
			}
		}
		return nil
	}

	const pass = "correct horse battery"
	cli := auth.NewService(auth.OpenStore(app.AuthFilePath(dataMgr.RootDir())), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig())
	code, _ := cli.IssueSetupCode()
	cookie := sessionOf(send("POST", "/admin/api/auth/setup", `{"code":"`+code+`","passphrase":"`+pass+`"}`, nil, ""))
	if cookie == nil {
		t.Fatal("setup did not sign in")
	}

	const ordinary = `{"model_host":"127.0.0.1"}`
	const runsCode = `{"default_args":["--host","0.0.0.0"]}`
	if rec := send("PUT", "/admin/api/config", ordinary, cookie, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("an ordinary save with a plain session = %d %s, want 204", rec.Code, rec.Body.String())
	}
	if rec := send("PUT", "/admin/api/config", runsCode, cookie, ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "reauth_required") {
		t.Errorf("changing the model arguments with a plain session = %d %s, want 403 reauth_required", rec.Code, rec.Body.String())
	}
	if rec := send("PUT", "/admin/api/system", runsCode, cookie, ""); rec.Code != http.StatusForbidden {
		t.Errorf("the same through /system = %d, want 403", rec.Code)
	}

	plain, _, err := cli.CreateToken("scripts", auth.ScopeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if rec := send("PUT", "/admin/api/config", ordinary, nil, plain); rec.Code != http.StatusNoContent {
		t.Errorf("a token may make an ordinary save: %d", rec.Code)
	}
	if rec := send("PUT", "/admin/api/config", runsCode, nil, plain); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "passphrase") {
		t.Errorf("a token changing the model arguments = %d %s, want 403", rec.Code, rec.Body.String())
	}

	elevated := sessionOf(send("POST", "/admin/api/auth/reauth", `{"passphrase":"`+pass+`"}`, cookie, ""))
	if elevated == nil {
		t.Fatal("reauth did not give a new cookie")
	}
	if rec := send("PUT", "/admin/api/config", runsCode, elevated, ""); rec.Code != http.StatusNoContent {
		t.Errorf("after confirming the passphrase = %d %s, want 204", rec.Code, rec.Body.String())
	}
}
