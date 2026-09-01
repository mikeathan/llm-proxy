package handlers_test

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
	"time"

	"fmt"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/testing/mocks"
	handlers "llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

func newProcessHandlers(runtime *mocks.MockManager, admin *mocks.MockAdminService, logger logging.Logger) *handlers.ProcessHandlers {
	return handlers.NewProcessHandlers(runtime, admin, logger)
}

// testWaitTimeout bounds async-probe assertions so a regression fails fast
// instead of hanging the suite.
const testWaitTimeout = 2 * time.Second

func TestAdminStartHandler_MissingName(t *testing.T) {
	procHandlers := newProcessHandlers(&mocks.MockManager{}, &mocks.MockAdminService{}, &mocks.MockLogger{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	procHandlers.AdminStartHandler(rr, req)

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
	procHandlers := newProcessHandlers(manager, &mocks.MockAdminService{}, &mocks.MockLogger{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	procHandlers.AdminStartHandler(rr, req)

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
	procHandlers := newProcessHandlers(manager, &mocks.MockAdminService{}, &mocks.MockLogger{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/start", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	procHandlers.AdminStartHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func newSystemHandlers(admin *mocks.MockAdminService) *handlers.SystemHandlers {
	if admin.GetSystemFunc == nil {
		admin.GetSystemFunc = func() models.SystemConfig { return models.SystemConfig{} }
	}
	if admin.GetRegistryFunc == nil {
		admin.GetRegistryFunc = func() models.RegistryData { return models.RegistryData{} }
	}
	return handlers.NewSystemHandlers(admin, &mocks.MockLogger{}, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"})
}

func TestAdminConfigUpdateHandler_InvalidJSON(t *testing.T) {
	handler := newSystemHandlers(&mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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
	handler := newSystemHandlers(admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader(`{"model_dir":"/tmp/models"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminConfigUpdateHandler_ModelNotFoundIsBadRequest(t *testing.T) {
	admin := &mocks.MockAdminService{
		ApplySystemUpdateFunc: func(ctx context.Context, req models.SystemUpdatePayload) error {
			return &models.ModelNotFoundError{Role: "primary", ModelName: "ghost"}
		},
	}
	handler := newSystemHandlers(admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/config", strings.NewReader(`{"primary_model":"ghost"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminConfigUpdateHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != `primary model "ghost" does not exist` {
		t.Fatalf("expected clear model-not-found message, got %q", resp["error"])
	}
}

func TestAdminConfigHandler_ServiceEnv(t *testing.T) {
	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")

	handler := newSystemHandlers(&mocks.MockAdminService{ServiceCredentialsFunc: func() (string, string) { return "client-id", "client-secret" }})

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
	// The service secret must never be emitted: it leaked the credential to
	// any client that could reach the API.
	if secret, ok := resp["service_client_secret"]; ok && secret != "" {
		t.Fatalf("service_client_secret must not be returned, got %v", secret)
	}
}

func TestAdminConfigHandler_ReturnsGPUMetrics(t *testing.T) {
	admin := &mocks.MockAdminService{}
	admin.GetSystemFunc = func() models.SystemConfig {
		return models.SystemConfig{
			Metrics: models.MetricsConfig{
				GPUSampleIntervalSec: 12,
				GPUSmoothingAlpha:    0.25,
			},
		}
	}
	handler := newSystemHandlers(admin)

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
	if got, ok := resp["gpu_sample_interval_seconds"]; !ok || got != float64(12) {
		t.Fatalf("expected gpu_sample_interval_seconds 12, got %v (present=%v)", got, ok)
	}
	if got, ok := resp["gpu_smoothing_alpha"]; !ok || got != 0.25 {
		t.Fatalf("expected gpu_smoothing_alpha 0.25, got %v (present=%v)", got, ok)
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
	handler := newSystemHandlers(admin)

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

func newModelHandlers(runtime *mocks.MockManager, admin *mocks.MockAdminService) *handlers.ModelHandlers {
	if runtime.ModelHostFunc == nil {
		runtime.ModelHostFunc = func() string { return "127.0.0.1" }
	}
	return handlers.NewModelHandlers(runtime, admin)
}

func TestAdminAddModelHandler_FileMissing(t *testing.T) {
	admin := &mocks.MockAdminService{
		ResolveModelPathFunc: func(filename string, path string) string {
			return filepath.Join(t.TempDir(), filename)
		},
	}
	handler := newModelHandlers(&mocks.MockManager{}, admin)

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
	handler := newModelHandlers(manager, admin)

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
	handler := newModelHandlers(manager, admin)

	body := `{"filename":"alpha.gguf","port":9001}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminAddModelHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminAddModelHandler_ProbeWarnOnly(t *testing.T) {
	var probedID *string
	manager := &mocks.MockManager{
		ListProviderModelsFunc: func(ctx context.Context, provider, apiKeyName string) ([]models.ProviderModelInfo, error) {
			return nil, nil
		},
		ClassifyModelFunc: func(cfg models.ModelConfig) models.WorkloadClass {
			return models.WorkloadCloud
		},
		AddModelFunc: func(cfg models.ModelConfig) error {
			return nil
		},
		ProbeModelAvailabilityFunc: func(ctx context.Context, cfg models.ModelConfig) error {
			probedID = &cfg.Filename
			return errors.New("model not callable: upstream status 404")
		},
	}
	handler := newModelHandlers(manager, &mocks.MockAdminService{})
	handler.ProbeLauncher = func(fn func()) { fn() }

	body := `{"name":"gpt-oss-120b","provider":"nvidia","model_id":"openai/gpt-oss-120b"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminAddModelHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 despite probe failure (warn-only), got %d: %s", rr.Code, rr.Body.String())
	}
	if probedID == nil {
		t.Fatal("expected ProbeModelAvailability to run for a cloud model add")
	}
	if *probedID != "openai/gpt-oss-120b" {
		t.Errorf("expected probe against the added model ID, got %q", *probedID)
	}
}

// TestAdminAddModelHandler_ProbeDoesNotBlockResponse verifies the availability
// probe runs in the background: the handler returns 200 while the probe is
// still in flight, and the probe runs to completion only after the handler has
// returned.
func TestAdminAddModelHandler_ProbeDoesNotBlockResponse(t *testing.T) {
	launcherCalled := make(chan struct{})
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})

	manager := &mocks.MockManager{
		ListProviderModelsFunc: func(ctx context.Context, provider, apiKeyName string) ([]models.ProviderModelInfo, error) {
			return nil, nil
		},
		ClassifyModelFunc: func(cfg models.ModelConfig) models.WorkloadClass {
			return models.WorkloadCloud
		},
		AddModelFunc: func(cfg models.ModelConfig) error {
			return nil
		},
		ProbeModelAvailabilityFunc: func(ctx context.Context, cfg models.ModelConfig) error {
			close(probeStarted)
			<-releaseProbe
			return errors.New("model not callable")
		},
	}
	handler := newModelHandlers(manager, &mocks.MockAdminService{})
	handler.ProbeLauncher = func(fn func()) {
		close(launcherCalled)
		go func() {
			fn()
		}()
	}

	body := `{"name":"gpt-oss","provider":"nvidia","model_id":"openai/gpt-oss"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.AdminAddModelHandler(rr, req)
	}()

	select {
	case <-launcherCalled:
	case <-time.After(testWaitTimeout):
		t.Fatal("probe launcher not invoked by the handler")
	}
	select {
	case <-probeStarted:
	case <-time.After(testWaitTimeout):
		t.Fatal("probe goroutine did not start")
	}

	// The probe is now blocked inside ProbeModelAvailability (releaseProbe is
	// not closed). If the handler returned here, it provably did not wait on it.
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatal("handler blocked on the availability probe; it must run in the background")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 while probe still in flight, got %d: %s", rr.Code, rr.Body.String())
	}

	close(releaseProbe)
	<-done
}

// TestAdminUpdateModelHandler_ProbeWarnOnly verifies an updated cloud model is
// probed (same warn-only, non-blocking semantics as add).
func TestAdminUpdateModelHandler_ProbeWarnOnly(t *testing.T) {
	var probedID *string
	manager := &mocks.MockManager{
		ListModelsFunc: func() []models.ModelConfig {
			return []models.ModelConfig{{Name: "gpt-oss", Provider: "nvidia", Filename: "old-id"}}
		},
		ClassifyModelFunc: func(cfg models.ModelConfig) models.WorkloadClass {
			return models.WorkloadCloud
		},
		UpdateModelFunc: func(cfg models.ModelConfig) error {
			return nil
		},
		ProbeModelAvailabilityFunc: func(ctx context.Context, cfg models.ModelConfig) error {
			probedID = &cfg.Filename
			return nil
		},
	}
	handler := newModelHandlers(manager, &mocks.MockAdminService{})
	handler.ProbeLauncher = func(fn func()) { fn() }

	body := `{"name":"gpt-oss","provider":"nvidia","model_id":"openai/gpt-oss-120b"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for update despite probe, got %d: %s", rr.Code, rr.Body.String())
	}
	if probedID == nil {
		t.Fatal("expected ProbeModelAvailability to run for a cloud model update")
	}
	if *probedID != "openai/gpt-oss-120b" {
		t.Errorf("expected probe against the updated model ID, got %q", *probedID)
	}
}

func TestAdminUpdateModelHandler_MissingName(t *testing.T) {
	handler := newModelHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

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
	handler := newModelHandlers(manager, &mocks.MockAdminService{})

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
	handler := newModelHandlers(manager, admin)

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
	handler := newModelHandlers(manager, admin)

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
	handler := newModelHandlers(manager, admin)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/models", strings.NewReader(`{"name":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AdminUpdateModelHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestAdminDeleteModelHandler_MissingName(t *testing.T) {
	handler := newModelHandlers(&mocks.MockManager{}, &mocks.MockAdminService{})

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
	handler := newModelHandlers(manager, &mocks.MockAdminService{})

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
	handler := newModelHandlers(manager, admin)

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
	if gemini.MaxTokens != 8192 {
		t.Errorf("expected gemini max_tokens 8192 (cloud tuning-table prefill matches the policy cap, H3), got %d", gemini.MaxTokens)
	}

	local, ok := resp.Config.ProviderDefaults["local"]
	if !ok {
		t.Fatal("expected provider_defaults.local")
	}
	if local.ContextBudget != 8000 {
		t.Errorf("expected local context_budget 8000, got %d", local.ContextBudget)
	}
}

// TestAdminStateHandler_ReasoningCapability verifies the reasoning capability
// descriptor is surfaced per provider via provider_defaults, and that local is
// non-toggleable (no UI toggle, no wire change).
func TestAdminStateHandler_ReasoningCapability(t *testing.T) {
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
			ProviderDefaults map[string]struct {
				Reasoning struct {
					Supported      bool   `json:"supported"`
					Toggleable     bool   `json:"toggleable"`
					DefaultEnabled bool   `json:"default_enabled"`
					Mode           string `json:"mode"`
				} `json:"reasoning"`
				SupportsBaseURL bool `json:"supports_base_url"`
			} `json:"provider_defaults"`
		} `json:"config"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	expect := map[string]struct {
		toggleable     bool
		defaultEnabled bool
		mode           string
		supportsBase   bool
	}{
		"openai":     {toggleable: true, defaultEnabled: true, mode: "effort", supportsBase: true},
		"gemini":     {toggleable: true, defaultEnabled: true, mode: "effort", supportsBase: false},
		"openrouter": {toggleable: true, defaultEnabled: true, mode: "object", supportsBase: true},
		"nvidia":     {toggleable: true, defaultEnabled: true, mode: "enable_thinking", supportsBase: true},
		"local":      {toggleable: false, defaultEnabled: false, mode: "think_tokens", supportsBase: false},
	}
	for k, want := range expect {
		pd, ok := resp.Config.ProviderDefaults[k]
		if !ok {
			t.Fatalf("expected provider_defaults.%s", k)
		}
		if pd.Reasoning.Toggleable != want.toggleable {
			t.Errorf("%s: toggleable got %v want %v", k, pd.Reasoning.Toggleable, want.toggleable)
		}
		if pd.Reasoning.DefaultEnabled != want.defaultEnabled {
			t.Errorf("%s: default_enabled got %v want %v", k, pd.Reasoning.DefaultEnabled, want.defaultEnabled)
		}
		if pd.Reasoning.Mode != want.mode {
			t.Errorf("%s: mode got %q want %q", k, pd.Reasoning.Mode, want.mode)
		}
		if pd.Reasoning.Supported != (pd.Reasoning.Toggleable || pd.Reasoning.DefaultEnabled) {
			t.Errorf("%s: supported must equal toggleable||default_enabled", k)
		}
		if pd.SupportsBaseURL != want.supportsBase {
			t.Errorf("%s: supports_base_url got %v want %v", k, pd.SupportsBaseURL, want.supportsBase)
		}
	}
}

// TestAdminStateHandler_LoopStrategyOptions verifies the backend-driven
// loop-strategy option list is surfaced from the strategy registry so the
// frontend dropdown never hardcodes it (adding a strategy = no UI edit).
func TestAdminStateHandler_LoopStrategyOptions(t *testing.T) {
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
			LoopStrategyOptions []string `json:"loop_strategy_options"`
		} `json:"config"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	want := []string{"evaluator_optimizer", "plan_execute", "react"}
	if len(resp.Config.LoopStrategyOptions) != len(want) {
		t.Fatalf("expected %d loop_strategy_options, got %v", len(want), resp.Config.LoopStrategyOptions)
	}
	for i := range want {
		if resp.Config.LoopStrategyOptions[i] != want[i] {
			t.Errorf("loop_strategy_options[%d] = %q, want %q", i, resp.Config.LoopStrategyOptions[i], want[i])
		}
	}
}

func TestAdminLogLevelHandler(t *testing.T) {
	logger := &mocks.MockLogger{}
	procHandlers := newProcessHandlers(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/log-level", nil)
	rr := httptest.NewRecorder()

	procHandlers.AdminLogLevelHandler(rr, req)

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
	procHandlers := newProcessHandlers(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/log-level", strings.NewReader(`{"level":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	procHandlers.AdminLogLevelUpdateHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAdminLogLevelUpdateHandler_SetsLevel(t *testing.T) {
	logger := &mocks.MockLogger{}
	procHandlers := newProcessHandlers(&mocks.MockManager{}, &mocks.MockAdminService{}, logger)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/log-level", strings.NewReader(`{"level":"DEBUG"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	procHandlers.AdminLogLevelUpdateHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func newAdminHandlers(runtime *mocks.MockManager, admin *mocks.MockAdminService) *handlers.AdminHandlers {
	if runtime.ModelHostFunc == nil {
		runtime.ModelHostFunc = func() string { return "127.0.0.1" }
	}
	if admin.GetSystemFunc == nil {
		admin.GetSystemFunc = func() models.SystemConfig { return models.SystemConfig{} }
	}
	if admin.GetRegistryFunc == nil {
		admin.GetRegistryFunc = func() models.RegistryData { return models.RegistryData{} }
	}
	return handlers.NewAdminHandlers(runtime, admin, &mocks.MockLogger{}, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"}, nil)

}
func newAdminHandlersWithLogger(runtime *mocks.MockManager, admin *mocks.MockAdminService, logger logging.Logger) *handlers.AdminHandlers {
	if admin.GetSystemFunc == nil {
		admin.GetSystemFunc = func() models.SystemConfig { return models.SystemConfig{} }
	}
	if admin.GetRegistryFunc == nil {
		admin.GetRegistryFunc = func() models.RegistryData { return models.RegistryData{} }
	}
	return handlers.NewAdminHandlers(runtime, admin, logger, &buildinfo.Info{Version: "v1", Commit: "c1", BuildDate: "d1"}, nil)
}
