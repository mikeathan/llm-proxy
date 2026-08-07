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
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/shell"
	"llm-proxy/internal/testing/mocks"
	handlers "llm-proxy/internal/transport/http/handlers"
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

type mockShellProvider struct {
	recycledID     string
	sessions       []models.TerminalSessionView
	shutdownCalled bool
}

func (m *mockShellProvider) GetOrCreate(ctx context.Context, workspaceID string, hostPath string, idleTimeout time.Duration, allowedEnvVars []string, pathExtensions []string) (shell.Terminal, error) {
	return nil, nil
}

func (m *mockShellProvider) Recycle(ctx context.Context, workspaceID string) {
	m.recycledID = workspaceID
}

func (m *mockShellProvider) ListSessions() []models.TerminalSessionView {
	return m.sessions
}

func (m *mockShellProvider) Shutdown() {
	m.shutdownCalled = true
}

func (m *mockShellProvider) PGID(workspaceID string) (int, bool) {
	return 0, false
}

// Helper to create valid server with optional config overrides
func createTestServer(t *testing.T, mgr llm.RuntimeManager, initialCfg *models.Config) *app.AppContext {
	dir := t.TempDir()
	t.Setenv("LLM_PROXY_CONFIG_DIR", dir)

	// 1. Create System Config (config.json)
	sys := models.SystemConfig{}
	sys.Server.Bind = ":0"
	sys.Server.ModelHost = "http://localhost"
	sys.Server.IdleTimeoutSecs = 10

	if initialCfg != nil {
		sys.Server.Bind = initialCfg.Server.Bind
		sys.Server.ModelHost = initialCfg.Server.ModelHost
		sys.Server.IdleTimeoutSecs = initialCfg.Server.IdleTimeoutSecs
	}

	sysData, _ := json.Marshal(sys)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), sysData, 0644)

	// 1.5 Create Settings (settings.yml)
	settings := models.UserSettings{}
	if initialCfg != nil {
		if local, ok := initialCfg.Providers["local"]; ok {
			settings.Local.ModelDir = local.ModelDir
			settings.Local.LlamaServerBinary = local.LlamaServerBinary
			settings.Local.DefaultArgs = local.DefaultArgs
		}
		if settings.Local.DefaultArgs == nil && len(initialCfg.Server.DefaultArgs) > 0 {
			settings.Local.DefaultArgs = initialCfg.Server.DefaultArgs
		}
	}
	// We use json for the mock since YamlStore supports json unmarshalling if the ext is wrong, or we can write a yaml manually
	// Actually, just let Store load it since we added yaml support, we should write valid yaml or json.
	// We can write it as json for simplicity in tests if we just name it settings.yml and it expects valid yaml, JSON is valid YAML!
	settingsData, _ := json.Marshal(settings)
	_ = os.WriteFile(filepath.Join(dir, "settings.yml"), settingsData, 0644)

	// 2. Create Registry (registry.json)
	reg := models.RegistryData{
		Providers: make(map[string]models.ProviderRegistryEntry),
		Catalogue: []models.ModelRegistryEntry{},
	}
	if initialCfg != nil {
		for id, p := range initialCfg.Providers {
			reg.Providers[id] = models.ProviderRegistryEntry{
				Type:    p.Type,
				BaseURL: p.BaseURL,
			}
		}
		for _, m := range initialCfg.Models {
			reg.Catalogue = append(reg.Catalogue, models.ModelRegistryEntry{
				Name:       m.Name,
				ModelID:    m.Filename,
				ProviderID: m.Provider,
				Port:       m.Port,
			})
		}
		reg.PrimaryModel = initialCfg.Server.PrimaryModel
		reg.FallbackModel = initialCfg.Server.FallbackModel
	}
	regData, _ := json.Marshal(reg)
	_ = os.WriteFile(filepath.Join(dir, "registry.json"), regData, 0644)

	// 3. Create Secrets (secrets.json)
	sec := models.SecretData{
		Version:      1,
		ProviderKeys: make(map[string][]models.SecretEntry),
	}
	secData, _ := json.Marshal(sec)
	_ = os.WriteFile(filepath.Join(dir, "secrets.json"), secData, 0644)

	dataMgr, err := storage.NewDataManager(dir)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}

	if err := dataMgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	return app.NewServer(mgr, dataMgr)
}

func TestEnsureModelProxyHandler_MissingHeader_NoDefault(t *testing.T) {
	srv := createTestServer(t, mocks.NewMockManager(), nil)
	handlers := handlers.NewProxyHandlers(srv.Runtime())

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
	handlers := handlers.NewProxyHandlers(srv.Runtime())

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
	handlers := handlers.NewProxyHandlers(srv.Runtime())

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

	restore := handlers.SetReverseProxyFactory(func(target string) http.Handler {
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
	handlers := handlers.NewProxyHandlers(srv.Runtime())

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

func TestEnsureModelProxyHandler_JSONBodyModel(t *testing.T) {
	mp := &mockProxy{}
	restore := handlers.SetReverseProxyFactory(func(target string) http.Handler {
		return mp
	})
	defer restore()

	var ensuredName string
	mgr := &mocks.MockManager{
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			ensuredName = name
			return llm.ModelInstance{Port: 9999}, nil
		},
		RecordActivityFunc: func(name string) {},
	}

	srv := createTestServer(t, mgr, nil)
	handlers := handlers.NewProxyHandlers(srv.Runtime())

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"json-model-test","messages":[]}`))
	w := httptest.NewRecorder()

	handlers.EnsureModelProxyHandler(w, req)

	if !mp.called {
		t.Fatalf("expected reverse proxy ServeHTTP to be called")
	}
	if ensuredName != "json-model-test" {
		t.Fatalf("expected ensured model name to be %q, got %q", "json-model-test", ensuredName)
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
	admin := handlers.NewAdminHandlers(srv.Runtime(), srv, &mocks.MockLogger{}, &buildinfo.Info{}, nil)
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
	procHandlers := handlers.NewProcessHandlers(srv.Runtime(), srv, &mocks.MockLogger{})
	req := httptest.NewRequest("POST", "/admin/api/start", strings.NewReader(`{"name":"gamma"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	procHandlers.AdminStartHandler(w, req)

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
	procHandlers := handlers.NewProcessHandlers(srv.Runtime(), srv, &mocks.MockLogger{})
	req := httptest.NewRequest("POST", "/admin/api/stop", nil)
	w := httptest.NewRecorder()

	procHandlers.AdminStopHandler(w, req)

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
	procHandlers := handlers.NewProcessHandlers(srv.Runtime(), srv, &mocks.MockLogger{})
	req := httptest.NewRequest("POST", "/admin/api/stop", nil)
	w := httptest.NewRecorder()

	procHandlers.AdminStopHandler(w, req)

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
	modelHandlers := handlers.NewModelHandlers(srv.Runtime(), srv)
	body := strings.NewReader(fmt.Sprintf(`{"name":"theta","filename":"%s","port":9999,"args":["--ctx-size","2048"]}`, filepath.Base(tmpFile.Name())))
	req := httptest.NewRequest("POST", "/admin/api/models", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	modelHandlers.AdminAddModelHandler(w, req)

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

	initialCfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha"}, {Name: "beta"}},
	}
	ctx := createTestServer(t, mgr, initialCfg)

	p, f := ctx.SelectModels()
	if p != "alpha" {
		t.Fatalf("expected alpha, got %s", p)
	}
	if f != "" {
		t.Fatalf("expected empty fallback, got %s", f)
	}
}

func TestAppContextResolveModelPath(t *testing.T) {
	ctx := createTestServer(t, mocks.NewMockManager(), &models.Config{
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

func TestAppContextUpdateSystem_Persists(t *testing.T) {
	// Setup
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &models.SystemConfig{}
	cfg.Server.Bind = ":0"
	cfg.Server.IdleTimeoutSecs = 1
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	// Create manager
	dataMgr, _ := storage.NewDataManager(dir)
	dataMgr.LoadAll()

	ctx := app.NewServer(mocks.NewMockManager(), dataMgr)

	if err := ctx.UpdateSystem(func(c *models.SystemConfig) {
		c.Server.Bind = ":9999"
		c.Server.IdleTimeoutSecs = 42
	}); err != nil {
		t.Fatalf("update system: %v", err)
	}

	// Verify persistence via new manager
	loadedMgr, _ := storage.NewDataManager(dir)
	if err := loadedMgr.LoadAll(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	loaded := loadedMgr.System().Get()

	if loaded.Server.Bind != ":9999" || loaded.Server.IdleTimeoutSecs != 42 {
		t.Fatalf("unexpected config: %+v", loaded.Server)
	}
}

func TestAppContextPersistModel_UpdatesExisting(t *testing.T) {
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha", Port: 8081}},
	}
	ctx := createTestServer(t, mocks.NewMockManager(), cfg)
	dir := ctx.RootDir()

	// Update existing model with a new port
	if err := ctx.PersistModel(models.ModelConfig{Name: "alpha", Port: 9999}); err != nil {
		t.Fatalf("persist model: %v", err)
	}
	// Add a new model
	if err := ctx.PersistModel(models.ModelConfig{Name: "beta", Port: 8082}); err != nil {
		t.Fatalf("persist model: %v", err)
	}

	loadedMgr, _ := storage.NewDataManager(dir)
	loadedMgr.LoadAll()
	loaded := loadedMgr.Registry().Get()

	if len(loaded.Catalogue) != 2 {
		t.Fatalf("expected 2 models, got %d", len(loaded.Catalogue))
	}

	alpha, ok := findModel(loaded.Catalogue, "alpha")
	if !ok || alpha.Port != 9999 {
		t.Fatalf("expected alpha to be updated with port 9999, got %+v", alpha)
	}
}

func TestAppContextPersistModel_IncludesArgs(t *testing.T) {
	ctx := createTestServer(t, mocks.NewMockManager(), nil)
	dir := ctx.RootDir()

	args := []string{"--ctx-size", "4096", "--parallel", "4"}
	cfg := models.ModelConfig{
		Name:     "custom",
		Filename: "model.gguf",
		Provider: "local",
		Args:     args,
		Port:     8081,
	}

	if err := ctx.PersistModel(cfg); err != nil {
		t.Fatalf("persist model: %v", err)
	}

	// Reload and verify
	loadedMgr, _ := storage.NewDataManager(dir)
	loadedMgr.LoadAll()
	loaded := loadedMgr.Registry().Get()

	m, ok := findModel(loaded.Catalogue, "custom")
	if !ok {
		t.Fatalf("expected custom model to be found")
	}

	if len(m.Args) != len(args) {
		t.Fatalf("expected %d args, got %d", len(args), len(m.Args))
	}
	for i, v := range args {
		if m.Args[i] != v {
			t.Errorf("arg[%d]: expected %s, got %s", i, v, m.Args[i])
		}
	}
}

func TestAppContextPersistReplaceModel(t *testing.T) {
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha", Port: 8081}},
	}
	ctx := createTestServer(t, mocks.NewMockManager(), cfg)
	dir := ctx.RootDir()

	if err := ctx.PersistReplaceModel(models.ModelConfig{Name: "alpha", Filename: "new.gguf", Port: 9999}); err != nil {
		t.Fatalf("persist replace: %v", err)
	}
	if err := ctx.PersistReplaceModel(models.ModelConfig{Name: "beta", Filename: "beta.gguf", Port: 8082}); err != nil {
		t.Fatalf("persist replace new: %v", err)
	}

	loadedMgr, _ := storage.NewDataManager(dir)
	loadedMgr.LoadAll()
	loaded := loadedMgr.Registry().Get()

	alpha, ok := findModel(loaded.Catalogue, "alpha")
	if !ok || alpha.ModelID != "new.gguf" || alpha.Port != 9999 {
		t.Fatalf("expected alpha ModelID new.gguf and Port 9999, got %+v", alpha)
	}
	beta, ok := findModel(loaded.Catalogue, "beta")
	if !ok || beta.Port != 8082 {
		t.Fatalf("expected beta to be added with port 8082, got %+v", beta)
	}
}

func TestAppContextPersistDeleteModel(t *testing.T) {
	cfg := &models.Config{
		Models: []models.ModelConfig{{Name: "alpha"}, {Name: "beta"}},
	}
	ctx := createTestServer(t, mocks.NewMockManager(), cfg)
	dir := ctx.RootDir()

	if err := ctx.PersistDeleteModel("alpha"); err != nil {
		t.Fatalf("persist delete: %v", err)
	}

	loadedMgr, _ := storage.NewDataManager(dir)
	loadedMgr.LoadAll()
	loaded := loadedMgr.Registry().Get()

	if _, ok := findModel(loaded.Catalogue, "alpha"); ok {
		t.Fatalf("expected alpha to be removed")
	}
	if _, ok := findModel(loaded.Catalogue, "beta"); !ok {
		t.Fatalf("expected beta to remain, got %d models", len(loaded.Catalogue))
	}
}

func TestAppContextUpdateSettings_Tools(t *testing.T) {
	dir := t.TempDir()
	dataMgr, _ := storage.NewDataManager(dir)
	_ = dataMgr.LoadAll()

	ctx := app.NewServer(mocks.NewMockManager(), dataMgr)

	// Update communication settings
	req := models.SystemUpdatePayload{
		Communication: &models.CommunicationConfig{
			Connectors: map[string]models.ConnectorConfig{
				"my-telegram": {
					Type:    "telegram",
					Enabled: true,
					Settings: map[string]string{
						"chat_id": "12345",
					},
					SecretRef: "my-telegram",
				},
			},
		},
	}

	if err := ctx.ApplySystemUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplySystemUpdate failed: %v", err)
	}

	// Verify it went to registry, NOT system
	reg := ctx.GetRegistry()
	cfg, ok := reg.Communication.Connectors["my-telegram"]
	if !ok || !cfg.Enabled || cfg.Settings["chat_id"] != "12345" {
		t.Errorf("expected registry to have telegram config, got %+v", reg.Communication)
	}

	// Verify persistence
	loadedMgr, _ := storage.NewDataManager(dir)
	_ = loadedMgr.LoadAll()
	loadedReg := loadedMgr.Registry().Get()
	loadedCfg, ok := loadedReg.Communication.Connectors["my-telegram"]
	if !ok || !loadedCfg.Enabled || loadedCfg.Settings["chat_id"] != "12345" {
		t.Errorf("persistence failed for registry tool config, got %+v", loadedReg.Communication)
	}
}

func TestAppContextConnectorWebhookURL_Persists(t *testing.T) {
	dir := t.TempDir()
	dataMgr, _ := storage.NewDataManager(dir)
	_ = dataMgr.LoadAll()

	ctx := app.NewServer(mocks.NewMockManager(), dataMgr)

	// Save connector with webhook URL
	req := models.SystemUpdatePayload{
		Communication: &models.CommunicationConfig{
			Connectors: map[string]models.ConnectorConfig{
				"my-telegram": {
					Type:       "telegram",
					Enabled:    true,
					Settings:   map[string]string{"chat_id": "12345"},
					SecretRef:  "my-telegram",
					WebhookURL: "https://example.com/api/v1/webhooks/my-telegram",
				},
			},
		},
	}
	if err := ctx.ApplySystemUpdate(context.Background(), req); err != nil {
		t.Fatalf("ApplySystemUpdate failed: %v", err)
	}

	// Verify in-memory
	reg := ctx.GetRegistry()
	cfg, ok := reg.Communication.Connectors["my-telegram"]
	if !ok || cfg.WebhookURL != "https://example.com/api/v1/webhooks/my-telegram" {
		t.Errorf("expected webhook URL in registry, got %+v", cfg)
	}

	// Verify persisted to disk
	loadedMgr, _ := storage.NewDataManager(dir)
	_ = loadedMgr.LoadAll()
	loadedCfg, ok := loadedMgr.Registry().Get().Communication.Connectors["my-telegram"]
	if !ok || loadedCfg.WebhookURL != "https://example.com/api/v1/webhooks/my-telegram" {
		t.Errorf("webhook URL not persisted, got %+v", loadedCfg)
	}

	// Clear webhook URL
	if err := ctx.UpdateRegistry(func(reg *models.RegistryData) {
		if c, ok := reg.Communication.Connectors["my-telegram"]; ok {
			c.WebhookURL = ""
			reg.Communication.Connectors["my-telegram"] = c
		}
	}); err != nil {
		t.Fatalf("UpdateRegistry failed: %v", err)
	}

	// Verify cleared
	reg = ctx.GetRegistry()
	cfg, _ = reg.Communication.Connectors["my-telegram"]
	if cfg.WebhookURL != "" {
		t.Errorf("expected webhook URL cleared, got %q", cfg.WebhookURL)
	}
}

func findModel(config []models.ModelRegistryEntry, name string) (models.ModelRegistryEntry, bool) {
	for _, m := range config {
		if m.Name == name {
			return m, true
		}
	}
	return models.ModelRegistryEntry{}, false
}

func TestAppContext_TerminalLifecycle(t *testing.T) {
	s := &app.AppContext{}
	mock := &mockShellProvider{
		sessions: []models.TerminalSessionView{
			{WorkspaceID: "ws-123", HostPath: "/tmp/ws-123"},
		},
	}

	s.SetShellProvider(mock)

	t.Run("ListShellSessions proxies to provider", func(t *testing.T) {
		sessions := s.ListShellSessions()
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		if sessions[0].WorkspaceID != "ws-123" {
			t.Errorf("expected ws-123, got %s", sessions[0].WorkspaceID)
		}
	})

	t.Run("ResetShell proxies Recycle to provider", func(t *testing.T) {
		err := s.ResetShell("target-ws")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mock.recycledID != "target-ws" {
			t.Errorf("expected target-ws to be recycled, got %s", mock.recycledID)
		}
	})

	t.Run("Shutdown proxies to provider", func(t *testing.T) {
		s.Shutdown()
		if !mock.shutdownCalled {
			t.Error("expected Shutdown to be called on provider")
		}
	})

	t.Run("ResetShell returns error when provider missing", func(t *testing.T) {
		s2 := &app.AppContext{}
		err := s2.ResetShell("ws")
		if err == nil {
			t.Error("expected error when terminal provider is nil")
		}
	})

}

func TestApplySystemUpdate_GPUMetricsFields(t *testing.T) {
	srv := createTestServer(t, mocks.NewMockManager(), nil)

	err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{
		GPUSampleIntervalSec: 7,
		GPUSmoothingAlpha:    0.15,
	})
	if err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}

	sys := srv.GetSystem()
	if sys.Metrics.GPUSampleIntervalSec != 7 {
		t.Fatalf("expected GPUSampleIntervalSec 7, got %d", sys.Metrics.GPUSampleIntervalSec)
	}
	if sys.Metrics.GPUSmoothingAlpha != 0.15 {
		t.Fatalf("expected GPUSmoothingAlpha 0.15, got %v", sys.Metrics.GPUSmoothingAlpha)
	}
}
