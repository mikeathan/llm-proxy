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
	}

	for _, path := range tests {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()

		a.Handler().ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s", path)
		}
	}
}

func TestMethodEnforcement(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalTestDataManager(t)
	a := app.New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
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
