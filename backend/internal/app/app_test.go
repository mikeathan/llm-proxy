package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
	req.Host = testHost
	rec := httptest.NewRecorder()

	a.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
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
