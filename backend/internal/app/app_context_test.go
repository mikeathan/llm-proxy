package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/testing/mocks"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

type mockProxy struct {
	called bool
}

func (m *mockProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.called = true
	w.WriteHeader(200)
	w.Write([]byte("proxied"))
}

// Helper to create valid server with optional config overrides
func createTestServer(t *testing.T, mgr llm.RuntimeManager, initialCfg *models.Config) *app.AppContext {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if initialCfg == nil {
		initialCfg = &models.Config{}
	}

	cfgMgr := config.NewConfigManager(configPath)

	data, err := json.Marshal(initialCfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := cfgMgr.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}

	return app.NewServer(mgr, cfgMgr)
}

func TestEnsureModelProxyHandler_MissingHeader_NoDefault(t *testing.T) {
	srv := createTestServer(t, &mocks.MockManager{}, nil)
	handlers := api.NewProxyHandlers(srv.Runtime())

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	handlers.EnsureModelProxyHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing model name and no default configured") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestEnsureModelProxyHandler_ModelStarting(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, models.ErrModelStarting
		},
	}

	srv := createTestServer(t, mgr, nil)
	handlers := api.NewProxyHandlers(srv.Runtime())

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	handlers.EnsureModelProxyHandler(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After missing")
	}
	if w.Header().Get("X-LLM-Status") != "starting" {
		t.Fatalf("X-LLM-Status missing")
	}
	if w.Body.String() != `{"status":"starting"}` {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestEnsureModelProxyHandler_ModelError(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, errors.New("boom")
		},
	}

	srv := createTestServer(t, mgr, nil)
	handlers := api.NewProxyHandlers(srv.Runtime())

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	handlers.EnsureModelProxyHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "model error") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestEnsureModelProxyHandler_ProxyCalled(t *testing.T) {
	// mock reverse proxy
	mp := &mockProxy{}

	restore := api.SetReverseProxyFactory(func(target string) http.Handler {
		return mp
	})
	defer restore()

	mgr := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{Port: 9999}, nil
		},
		RecordActivityFunc: func(name string) {},
	}

	srv := createTestServer(t, mgr, nil)
	handlers := api.NewProxyHandlers(srv.Runtime())

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Model-Name", "test")

	w := httptest.NewRecorder()

	handlers.EnsureModelProxyHandler(w, req)

	if !mp.called {
		t.Fatalf("expected reverse proxy ServeHTTP to be called")
	}

	if w.Code != 200 || w.Body.String() != "proxied" {
		t.Fatalf("unexpected proxy output: %d %s", w.Code, w.Body.String())
	}
}

func TestAdminStateHandler(t *testing.T) {
	now := time.Now()
	mgr := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{
				{Name: "alpha", Filename: "alpha.gguf", Path: "/models/alpha.gguf", Port: 8081},
				{Name: "beta", Filename: "beta.gguf", Path: "/models/beta.gguf", Port: 8082},
			}
		},
		ActiveInfoFunc: func() *llm.ActiveModelInfo {
			return &llm.ActiveModelInfo{
				Name:     "alpha",
				Port:     8081,
				Host:     "127.0.0.1",
				Ready:    true,
				Started:  now,
				LastUsed: now,
			}
		},
		ModelHostFunc: func() string { return "127.0.0.1" },
	}

	srv := createTestServer(t, mgr, nil)
	admin := api.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{})
	req := httptest.NewRequest("GET", "/admin/api/state", nil)
	w := httptest.NewRecorder()

	admin.AdminStateHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Models []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Ready  bool   `json:"ready"`
		} `json:"models"`
		Active struct {
			Name string `json:"name"`
		} `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}
	if resp.Active.Name != "alpha" {
		t.Fatalf("expected active model alpha, got %s", resp.Active.Name)
	}
	var activeRow struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
		Ready  bool   `json:"ready"`
	}
	for _, m := range resp.Models {
		if m.Name == "alpha" {
			activeRow = m
		}
	}
	if !activeRow.Active || !activeRow.Ready {
		t.Fatalf("alpha should be active and ready")
	}
}

func TestAdminStartHandler(t *testing.T) {
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{Name: name, Port: 9090}, nil
		},
		RecordActivityFunc: func(name string) {},
		ModelHostFunc:      func() string { return "127.0.0.1" },
	}

	srv := createTestServer(t, mgr, nil)
	admin := api.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{})
	req := httptest.NewRequest("POST", "/admin/api/start", strings.NewReader(`{"name":"gamma"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	admin.AdminStartHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"gamma"`) {
		t.Fatalf("response should mention model name, got %s", w.Body.String())
	}
}

func TestAdminStopHandler(t *testing.T) {
	stopped := false
	mgr := &mocks.MockManager{
		ActiveInfoFunc: func() *llm.ActiveModelInfo { return &llm.ActiveModelInfo{Name: "delta"} },
		StopActiveFunc: func() error {
			stopped = true
			return nil
		},
	}

	srv := createTestServer(t, mgr, nil)
	admin := api.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{})
	req := httptest.NewRequest("POST", "/admin/api/stop", nil)
	w := httptest.NewRecorder()

	admin.AdminStopHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !stopped {
		t.Fatalf("StopActive should be called")
	}
	if !strings.Contains(w.Body.String(), "delta") {
		t.Fatalf("expected response to include stopped model, got %s", w.Body.String())
	}
}

func TestAdminStopHandler_Error(t *testing.T) {
	mgr := &mocks.MockManager{
		ActiveInfoFunc: func() *llm.ActiveModelInfo { return &llm.ActiveModelInfo{Name: "delta"} },
		StopActiveFunc: func() error { return errors.New("boom") },
	}

	srv := createTestServer(t, mgr, nil)
	admin := api.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{})
	req := httptest.NewRequest("POST", "/admin/api/stop", nil)
	w := httptest.NewRecorder()

	admin.AdminStopHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("expected error message in body, got %s", w.Body.String())
	}
}

func TestAdminAddModelHandler(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "model-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp model: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	var added models.ModelConfig
	mgr := &mocks.MockManager{
		AddModelFunc: func(cfg models.ModelConfig) error {
			added = cfg
			return nil
		},
		ListModelsFunc: func() []models.ModelConfig { return nil },
	}

	cfg := &models.Config{
		Server: models.ServerConfig{DefaultArgs: []string{"--gpu-layers", "2"}},
		Providers: map[string]models.ProviderItem{
			"local": {ModelDir: filepath.Dir(tmpFile.Name())},
		},
	}

	srv := createTestServer(t, mgr, cfg)
	admin := api.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{})
	body := strings.NewReader(fmt.Sprintf(`{"name":"theta","filename":"%s","port":9999,"args":["--ctx-size","2048"]}`, filepath.Base(tmpFile.Name())))
	req := httptest.NewRequest("POST", "/admin/api/models", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	admin.AdminAddModelHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if added.Name != "theta" || added.Port != 9999 {
		t.Fatalf("unexpected model added: %+v", added)
	}
	if added.Filename != filepath.Base(tmpFile.Name()) || added.Path != tmpFile.Name() {
		t.Fatalf("expected resolved path to match temp file, got %+v", added)
	}
	if len(added.Args) != 2 || added.Args[0] != "--ctx-size" || added.Args[1] != "2048" {
		t.Fatalf("expected args to not merge defaults if provided, got %v", added.Args)
	}
}

// --- AppContext behavior tests ---

func TestAppContextSelectModels_NoModels(t *testing.T) {
	mgr := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig { return nil },
	}
	ctx := createTestServer(t, mgr, nil)

	p, f := ctx.SelectModels()
	if p != "" || f != "" {
		t.Fatalf("expected empty models when none configured")
	}
}

func TestAppContextSelectModels_FirstModel(t *testing.T) {
	mgr := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{{Name: "alpha"}, {Name: "beta"}}
		},
	}
	ctx := createTestServer(t, mgr, nil)

	p, f := ctx.SelectModels()
	if p != "alpha" {
		t.Fatalf("expected alpha, got %s", p)
	}
	if f != "" {
		t.Fatalf("expected empty fallback, got %s", f)
	}
}

func TestAppContextResolveModelPath(t *testing.T) {
	ctx := createTestServer(t, &mocks.MockManager{}, &models.Config{
		Providers: map[string]models.ProviderItem{
			"local": {ModelDir: "/models"},
		},
	})

	cases := []struct {
		name     string
		filename string
		explicit string
		modelDir string
		want     string
	}{
		{name: "explicit absolute wins", filename: "ignored.gguf", explicit: "/abs/model.gguf", modelDir: "/models", want: "/abs/model.gguf"},
		{name: "filename absolute wins", filename: "/abs/other.gguf", explicit: "", modelDir: "/models", want: "/abs/other.gguf"},
		{name: "joins model dir", filename: "foo.gguf", explicit: "", modelDir: "/models", want: "/models/foo.gguf"},
		{name: "explicit non-abs with empty filename", filename: "", explicit: "rel.gguf", modelDir: "/models", want: "rel.gguf"},
		{name: "no model dir falls back to filename", filename: "bar.gguf", explicit: "", modelDir: "", want: "bar.gguf"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx.SetModelDir(tc.modelDir)
			got := ctx.ResolveModelPath(tc.filename, tc.explicit)
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestAppContextUpdateConfig_Persists(t *testing.T) {
	// Setup
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &models.Config{
		Server: models.ServerConfig{Bind: ":0", IdleTimeoutSecs: 1},
		Models: []models.ModelConfig{},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	// Create manager
	mgr := config.NewConfigManager(path)
	mgr.Load()

	ctx := app.NewServer(&mocks.MockManager{}, mgr)

	if err := ctx.UpdateConfig(func(c *models.Config) {
		c.Server.Bind = ":9999"
		c.Server.IdleTimeoutSecs = 42
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	// Verify persistence via new manager (avoiding stale cache if any, though load reads file)
	loadedMgr := config.NewConfigManager(path)
	if err := loadedMgr.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	loaded := loadedMgr.GetConfig()

	if loaded.Server.Bind != ":9999" || loaded.Server.IdleTimeoutSecs != 42 {
		t.Fatalf("unexpected config: %+v", loaded.Server)
	}
}

func TestAppContextPersistModel_AddsOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha"}},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	mgr := config.NewConfigManager(path)
	mgr.Load()

	ctx := app.NewServer(&mocks.MockManager{}, mgr)

	if err := ctx.PersistModel(models.ModelConfig{Name: "beta"}); err != nil {
		t.Fatalf("persist model: %v", err)
	}
	if err := ctx.PersistModel(models.ModelConfig{Name: "alpha"}); err != nil {
		t.Fatalf("persist model (duplicate): %v", err)
	}

	loadedMgr := config.NewConfigManager(path)
	loadedMgr.Load()
	loaded := loadedMgr.GetConfig()

	if len(loaded.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(loaded.Models))
	}
}

func TestAppContextPersistReplaceModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha", Port: 1}},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	mgr := config.NewConfigManager(path)
	mgr.Load()

	ctx := app.NewServer(&mocks.MockManager{}, mgr)

	if err := ctx.PersistReplaceModel(models.ModelConfig{Name: "alpha", Port: 9}); err != nil {
		t.Fatalf("persist replace: %v", err)
	}
	if err := ctx.PersistReplaceModel(models.ModelConfig{Name: "beta", Port: 2}); err != nil {
		t.Fatalf("persist replace new: %v", err)
	}

	loadedMgr := config.NewConfigManager(path)
	loadedMgr.Load()
	loaded := loadedMgr.GetConfig()

	alpha, ok := findModel(loaded.Models, "alpha")
	if !ok || alpha.Port != 9 {
		t.Fatalf("expected alpha port 9, got %+v", alpha)
	}
	if _, ok := findModel(loaded.Models, "beta"); !ok {
		t.Fatalf("expected beta to be added")
	}
}

func TestAppContextPersistDeleteModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha"}, {Name: "beta"}},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	mgr := config.NewConfigManager(path)
	mgr.Load()

	ctx := app.NewServer(&mocks.MockManager{}, mgr)

	if err := ctx.PersistDeleteModel("alpha"); err != nil {
		t.Fatalf("persist delete: %v", err)
	}

	loadedMgr := config.NewConfigManager(path)
	loadedMgr.Load()
	loaded := loadedMgr.GetConfig()

	if _, ok := findModel(loaded.Models, "alpha"); ok {
		t.Fatalf("expected alpha to be removed")
	}
	if _, ok := findModel(loaded.Models, "beta"); !ok {
		t.Fatalf("expected beta to remain")
	}
}

func findModel(config []models.ModelConfig, name string) (models.ModelConfig, bool) {
	for _, m := range config {
		if m.Name == name {
			return m, true
		}
	}

	return models.ModelConfig{}, false
}
