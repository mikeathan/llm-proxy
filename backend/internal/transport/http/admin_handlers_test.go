package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fmt"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/testing/mocks"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

func TestAdminStartHandler_MissingName(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
			return llm.ModelInstance{}, models.ErrModelStarting
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
		EnsureModelFunc: func(ctx context.Context, name string) (llm.ModelInstance, error) {
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
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminConfigUpdateHandler_UpdateSystemError(t *testing.T) {
	admin := &mocks.MockAdminService{
		ApplySystemUpdateFunc: func(ctx context.Context, req models.SystemUpdatePayload) error {
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

	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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

	admin := &mocks.MockAdminService{
		ApplySystemUpdateFunc: func(ctx context.Context, req models.SystemUpdatePayload) error {
			if req.ServiceClientID != "" {
				os.Setenv("SERVICE_CLIENT_ID", req.ServiceClientID)
			}
			if req.ServiceClientSecret != "" {
				os.Setenv("SERVICE_CLIENT_SECRET", req.ServiceClientSecret)
			}

			envUpdates := map[string]string{}
			if req.ServiceClientID != "" {
				envUpdates["SERVICE_CLIENT_ID"] = req.ServiceClientID
			}
			if req.ServiceClientSecret != "" {
				envUpdates["SERVICE_CLIENT_SECRET"] = req.ServiceClientSecret
			}

			if len(envUpdates) > 0 {
				exe, _ := os.Executable()
				exe, _ = filepath.EvalSymlinks(exe)
				envPath := filepath.Join(filepath.Dir(exe), ".env")
				data := ""
				for k, v := range envUpdates {
					data += fmt.Sprintf("%s=%q\n", k, v)
				}
				_ = os.WriteFile(envPath, []byte(data), 0644)
			}
			return nil
		},
	}
	handler := newAdminHandlers(&mocks.MockManager{}, admin)

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

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	envPath := filepath.Join(filepath.Dir(exe), ".env")
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
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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

func TestAdminStateHandler_AgentDefaults(t *testing.T) {
	handler := newAdminHandlers(&mocks.MockManager{}, &mocks.MockAdminService{
		GetGuardrailsFunc: func() models.AgentGuardrailsConfig {
			return models.AgentGuardrailsConfig{}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/state", nil)
	rr := httptest.NewRecorder()
	handler.AdminStateHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Config struct {
			AgentDefaults struct {
				MaxSteps       int    `json:"max_steps"`
				ContextBudget  int    `json:"context_budget"`
				MaxTokens      int    `json:"max_tokens"`
				ToolCallFormat string `json:"tool_call_format"`
				Prefill        bool   `json:"prefill"`
			} `json:"agent_defaults"`
			ProviderDefaults map[string]struct {
				MaxSteps       int    `json:"max_steps"`
				ContextBudget  int    `json:"context_budget"`
				MaxTokens      int    `json:"max_tokens"`
				ToolCallFormat string `json:"tool_call_format"`
				Prefill        bool   `json:"prefill"`
			} `json:"provider_defaults"`
		} `json:"config"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Config.AgentDefaults.MaxSteps != 25 {
		t.Errorf("expected max_steps 25, got %d", resp.Config.AgentDefaults.MaxSteps)
	}
	if resp.Config.AgentDefaults.ContextBudget != 8000 {
		t.Errorf("expected context_budget 8000, got %d", resp.Config.AgentDefaults.ContextBudget)
	}
	if resp.Config.AgentDefaults.MaxTokens != 3072 {
		t.Errorf("expected max_tokens 3072, got %d", resp.Config.AgentDefaults.MaxTokens)
	}
	if resp.Config.AgentDefaults.Prefill {
		t.Errorf("expected prefill false, got true")
	}

	gemini, ok := resp.Config.ProviderDefaults["gemini"]
	if !ok {
		t.Fatal("expected provider_defaults.gemini")
	}
	if gemini.ContextBudget != 50000 {
		t.Errorf("expected gemini context_budget 50000, got %d", gemini.ContextBudget)
	}
	if gemini.MaxSteps != 35 {
		t.Errorf("expected gemini max_steps 35, got %d", gemini.MaxSteps)
	}
	if gemini.MaxTokens != 4096 {
		t.Errorf("expected gemini max_tokens 4096, got %d", gemini.MaxTokens)
	}

	local, ok := resp.Config.ProviderDefaults["local"]
	if !ok {
		t.Fatal("expected provider_defaults.local")
	}
	if local.ContextBudget != 8000 {
		t.Errorf("expected local context_budget 8000, got %d", local.ContextBudget)
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
	if admin.GetSystemFunc == nil {
		admin.GetSystemFunc = func() models.SystemConfig { return models.SystemConfig{} }
	}
	if admin.GetRegistryFunc == nil {
		admin.GetRegistryFunc = func() models.RegistryData { return models.RegistryData{} }
	}
	return api.NewAdminHandlers(runtime, admin, &mocks.MockLogger{}, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"})
}

func newAdminHandlersWithLogger(runtime *mocks.MockManager, admin *mocks.MockAdminService, logger *mocks.MockLogger) *api.AdminHandlers {
	if runtime.ModelHostFunc == nil {
		runtime.ModelHostFunc = func() string { return "127.0.0.1" }
	}
	if admin.GetSystemFunc == nil {
		admin.GetSystemFunc = func() models.SystemConfig { return models.SystemConfig{} }
	}
	if admin.GetRegistryFunc == nil {
		admin.GetRegistryFunc = func() models.RegistryData { return models.RegistryData{} }
	}
	return api.NewAdminHandlers(runtime, admin, logger, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"})
}
