package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/models"
)

// fakeRuntime implements RuntimeService with only the methods the proxy
// handlers use wired; every other method is a stub.
type fakeRuntime struct {
	models      []models.ModelConfig
	activeInfo  *llm.ActiveModelInfo
	instance    llm.ModelInstance
	ensureErr   error
	ensureCalls int
}

func (f *fakeRuntime) ListModels() []models.ModelConfig { return f.models }
func (f *fakeRuntime) EnsureModel(context.Context, string) (llm.ModelInstance, error) {
	f.ensureCalls++
	return f.instance, f.ensureErr
}
func (f *fakeRuntime) RecordActivity(string)                     {}
func (f *fakeRuntime) Sync()                                     {}
func (f *fakeRuntime) AddModel(models.ModelConfig) error         { return nil }
func (f *fakeRuntime) UpdateModel(models.ModelConfig) error      { return nil }
func (f *fakeRuntime) RemoveModel(string) error                  { return nil }
func (f *fakeRuntime) ActiveInfo() *llm.ActiveModelInfo          { return f.activeInfo }
func (f *fakeRuntime) ActiveLogs() string                        { return "" }
func (f *fakeRuntime) LastTokensPerSecond() (float64, time.Time) { return 0, time.Time{} }
func (f *fakeRuntime) LastModelError() string                    { return "" }
func (f *fakeRuntime) StopActive() error                         { return nil }
func (f *fakeRuntime) ClearLogs() error                          { return nil }
func (f *fakeRuntime) ModelHost() string                         { return "" }
func (f *fakeRuntime) SetModelHost(string)                       {}
func (f *fakeRuntime) ListProviderModels(context.Context, string, string) ([]models.ProviderModelInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) TestProviderConnection(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRuntime) ProbeModelAvailability(context.Context, models.ModelConfig) error { return nil }
func (f *fakeRuntime) SelectModels() (string, string)                                   { return "", "" }
func (f *fakeRuntime) RefreshServingFingerprints(context.Context)                       {}
func (f *fakeRuntime) ApplyModelOverrides(map[string]models.ModelOverride)              {}
func (f *fakeRuntime) ClassifyModel(models.ModelConfig) models.WorkloadClass            { return "" }

func TestModelsListHandler_ServesMetadata(t *testing.T) {
	rt := &fakeRuntime{models: []models.ModelConfig{
		// Serving window comes from the launch args (--ctx-size), training
		// context from the metadata/registry.
		{Name: "Qwen3.6-35B-A3B", Provider: models.ProviderLocal, MaxTokens: 87381, Args: []string{"-m", "/models/Qwen.gguf", "--ctx-size", "8192", "--threads", "8"}},
		// No --ctx-size → falls back to the training context.
		{Name: "Gpt Oss 20b", Provider: models.ProviderLocal, MaxTokens: 43690, Metadata: &models.ModelMetadata{ContextLength: 65536}},
	}}
	h := NewProxyHandlers(rt)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ModelsListHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("expected object=list, got %q", resp.Object)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data))
	}

	first := resp.Data[0]
	if first["id"] != "Qwen3.6-35B-A3B" {
		t.Errorf("expected id Qwen3.6-35B-A3B, got %v", first["id"])
	}
	if first["owned_by"] != "llamacpp" {
		t.Errorf("owned_by must be llamacpp to keep the client's local-workload fingerprint, got %v", first["owned_by"])
	}
	if first["max_tokens"] != float64(87381) || first["max_completion_tokens"] != float64(87381) {
		t.Errorf("expected max tokens 87381, got max_tokens=%v max_completion_tokens=%v", first["max_tokens"], first["max_completion_tokens"])
	}
	meta, ok := first["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %T", first["meta"])
	}
	if meta["n_ctx"] != float64(8192) {
		t.Errorf("serving context must come from --ctx-size (8192), got %v", meta["n_ctx"])
	}
	if meta["context_length"] != float64(8192) {
		t.Errorf("context_length must mirror the serving window, got %v", meta["context_length"])
	}
	if meta["n_ctx_train"] != float64(87381) {
		t.Errorf("expected n_ctx_train 87381, got %v", meta["n_ctx_train"])
	}

	secondMeta, ok := resp.Data[1]["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object on second model, got %T", resp.Data[1]["meta"])
	}
	if secondMeta["n_ctx"] != float64(65536) {
		t.Errorf("no --ctx-size → n_ctx falls back to training context, got %v", secondMeta["n_ctx"])
	}
}

// The listing is what a client uses to decide a model is a local llama.cpp
// workload, so a cloud model this proxy fronts must not be stamped as one.
func TestModelsListHandler_CloudModelIsNotLlamaCpp(t *testing.T) {
	rt := &fakeRuntime{models: []models.ModelConfig{
		{Name: "glm-5.3-flash", Provider: models.ProviderOpenRouter, MaxTokens: 131072, Metadata: &models.ModelMetadata{ContextLength: 1310720, Nctx: 1048576}},
	}}
	h := NewProxyHandlers(rt)

	rec := httptest.NewRecorder()
	h.ModelsListHandler(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Data) != 1 {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	entry := resp.Data[0]
	if entry["owned_by"] != models.ProviderOpenRouter {
		t.Errorf("owned_by = %v, want the provider (%s)", entry["owned_by"], models.ProviderOpenRouter)
	}
	meta, _ := entry["meta"].(map[string]any)
	if _, ok := meta["n_ctx_train"]; ok {
		t.Errorf("cloud entry carries n_ctx_train: %v", meta)
	}
	if entry["max_tokens"] != float64(131072) {
		t.Errorf("max_tokens = %v, want the published output cap", entry["max_tokens"])
	}
}

func TestServingCtxFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"ctx-size flag", []string{"-m", "x.gguf", "--ctx-size", "8192"}, 8192},
		{"short -c flag", []string{"-c", "4096"}, 4096},
		{"context-size flag", []string{"--context-size", "2048"}, 2048},
		{"no ctx flag", []string{"-m", "x.gguf", "--threads", "8"}, 0},
		{"missing value", []string{"--ctx-size"}, 0},
		{"nil args", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := servingCtxFromArgs(c.args); got != c.want {
				t.Fatalf("expected %d, got %d", c.want, got)
			}
		})
	}
}
