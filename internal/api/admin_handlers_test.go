package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/api"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/mocks"
	"llm-proxy/models"
)

func TestAdminStartHandler_MissingName(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminStartHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminStartHandler_ModelStarting(t *testing.T) {
	manager := &mocks.MockManager{
		EnsureModelFunc: func(name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, llm.ErrModelStarting
		},
	}
	handler := newAdminHandlers(manager, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminStartHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
}

func TestAdminStartHandler_ModelError(t *testing.T) {
	manager := &mocks.MockManager{
		EnsureModelFunc: func(name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, errors.New("boom")
		},
	}
	handler := newAdminHandlers(manager, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminStartHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminConfigUpdateHandler_InvalidJSON(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminConfigUpdateHandler_UpdateConfigError(t *testing.T) {
	admin := &mocks.MockAdminService{
		UpdateConfigFunc: func(func(*models.Config)) error {
			return errors.New("save failed")
		},
	}
	handler := newAdminHandlers(&mocks.MockManager{}, admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader(`{"model_dir":"/tmp/models"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminConfigHandler_ServiceEnv(t *testing.T) {
	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/config", nil)
	rr := httptest.NewRecorder()

	handler.AdminConfigHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["service_client_id"] != "client-id" {
		t.Fatalf("expected service_client_id, got %v", resp["service_client_id"])
	}
	if resp["service_client_secret"] != "client-secret" {
		t.Fatalf("expected service_client_secret, got %v", resp["service_client_secret"])
	}
}

func TestAdminConfigUpdateHandler_SetsServiceEnv(t *testing.T) {
	t.Setenv("SERVICE_CLIENT_ID", "")
	t.Setenv("SERVICE_CLIENT_SECRET", "")

	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	body := `{"service_client_id":"new-id","service_client_secret":"new-secret"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}

	if got := os.Getenv("SERVICE_CLIENT_ID"); got != "new-id" {
		t.Fatalf("expected SERVICE_CLIENT_ID to be set, got %q", got)
	}
	if got := os.Getenv("SERVICE_CLIENT_SECRET"); got != "new-secret" {
		t.Fatalf("expected SERVICE_CLIENT_SECRET to be set, got %q", got)
	}

	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(data), `SERVICE_CLIENT_ID="new-id"`) {
		t.Fatalf("expected SERVICE_CLIENT_ID in .env, got %s", string(data))
	}
	if !strings.Contains(string(data), `SERVICE_CLIENT_SECRET="new-secret"`) {
		t.Fatalf("expected SERVICE_CLIENT_SECRET in .env, got %s", string(data))
	}
}

func TestAdminAddModelHandler_FileMissing(t *testing.T) {
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return filepath.Join(t.TempDir(), filename)
		},
	}
	handler := newAdminHandlers(&mocks.MockManager{}, admin)

	body := `{"filename":"missing.gguf","port":9001}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminAddModelHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminAddModelHandler_ModelExists(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "alpha.gguf")
	if err := os.WriteFile(modelPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	manager := &mocks.MockManager{
		AddModelFunc: func(cfg models.ModelConfig) error {
			return llm.ErrModelExists
		},
	}
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return modelPath
		},
	}
	handler := newAdminHandlers(manager, admin)

	body := `{"filename":"alpha.gguf","port":9001}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminAddModelHandler(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestAdminAddModelHandler_PersistError(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "alpha.gguf")
	if err := os.WriteFile(modelPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	manager := &mocks.MockManager{
		AddModelFunc: func(cfg models.ModelConfig) error {
			return nil
		},
	}
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return modelPath
		},
		PersistModelFunc: func(cfg models.ModelConfig) error {
			return errors.New("persist failed")
		},
	}
	handler := newAdminHandlers(manager, admin)

	body := `{"filename":"alpha.gguf","port":9001}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminAddModelHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminUpdateModelHandler_MissingName(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminUpdateModelHandler_UnknownModel(t *testing.T) {
	manager := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{}
		},
	}
	handler := newAdminHandlers(manager, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{"name":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAdminUpdateModelHandler_FileMissing(t *testing.T) {
	manager := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{{Name: "alpha", Filename: "alpha.gguf", Port: 9001}}
		},
	}
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return filepath.Join(t.TempDir(), filename)
		},
	}
	handler := newAdminHandlers(manager, admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminUpdateModelHandler_RuntimeUnknown(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "alpha.gguf")
	if err := os.WriteFile(modelPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	manager := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{{Name: "alpha", Filename: "alpha.gguf", Port: 9001}}
		},
		UpdateModelFunc: func(cfg models.ModelConfig) error {
			return llm.ErrUnknownModel
		},
	}
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return modelPath
		},
	}
	handler := newAdminHandlers(manager, admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAdminUpdateModelHandler_PersistError(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "alpha.gguf")
	if err := os.WriteFile(modelPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	manager := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{{Name: "alpha", Filename: "alpha.gguf", Port: 9001}}
		},
		UpdateModelFunc: func(cfg models.ModelConfig) error {
			return nil
		},
	}
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return modelPath
		},
		PersistReplaceModelFunc: func(cfg models.ModelConfig) error {
			return errors.New("persist failed")
		},
	}
	handler := newAdminHandlers(manager, admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminDeleteModelHandler_MissingName(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/models", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminDeleteModelHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminDeleteModelHandler_UnknownModel(t *testing.T) {
	manager := &mocks.MockManager{
		RemoveModelFunc: func(name string) error {
			return llm.ErrUnknownModel
		},
	}
	handler := newAdminHandlers(manager, &mocks.MockAdminService{})

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/models?name=missing", nil)
	rr := httptest.NewRecorder()

	handler.AdminDeleteModelHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAdminDeleteModelHandler_PersistError(t *testing.T) {
	manager := &mocks.MockManager{
		RemoveModelFunc: func(name string) error {
			return nil
		},
	}
	admin := &mocks.MockAdminService{
		PersistDeleteModelFunc: func(name string) error {
			return errors.New("persist failed")
		},
	}
	handler := newAdminHandlers(manager, admin)

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/models?name=alpha", nil)
	rr := httptest.NewRecorder()

	handler.AdminDeleteModelHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminLogLevelHandler(t *testing.T) {
	logger := &mocks.MockLogger{}
	handler := newAdminHandlersWithLogger(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/log-level", nil)
	rr := httptest.NewRecorder()

	handler.AdminLogLevelHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}

	if resp["level"] == "" {
		t.Fatalf("expected level in response")
	}
}

func TestAdminLogLevelUpdateHandler_InvalidLevel(t *testing.T) {
	logger := &mocks.MockLogger{}
	handler := newAdminHandlersWithLogger(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/log-level", strings.NewReader(`{"level":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminLogLevelUpdateHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminLogLevelUpdateHandler_SetsLevel(t *testing.T) {
	logger := &mocks.MockLogger{}
	handler := newAdminHandlersWithLogger(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/log-level", strings.NewReader(`{"level":"DEBUG"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminLogLevelUpdateHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func newAdminHandlers(runtime *mocks.MockManager, admin *mocks.MockAdminService) *api.AdminHandlers {
	if runtime.ModelHostFunc == nil {
		runtime.ModelHostFunc = func() string { return "127.0.0.1" }
	}
	return api.NewAdminHandlers(runtime, admin, &mocks.MockLogger{}, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"})
}

func newAdminHandlersWithLogger(runtime *mocks.MockManager, admin *mocks.MockAdminService, logger *mocks.MockLogger) *api.AdminHandlers {
	if runtime.ModelHostFunc == nil {
		runtime.ModelHostFunc = func() string { return "127.0.0.1" }
	}
	return api.NewAdminHandlers(runtime, admin, logger, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"})
}
