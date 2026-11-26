package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/modelstats"
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
	// classify, when set, answers ClassifyModel (the runtime's hydrated classifier).
	classify func(models.ModelConfig) models.WorkloadClass
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
func (f *fakeRuntime) ClassifyModel(cfg models.ModelConfig) models.WorkloadClass {
	if f.classify != nil {
		return f.classify(cfg)
	}
	return ""
}

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

// A cloud target already holds the full endpoint, so the proxy blanks the incoming
// path; the upstream must then see that endpoint exactly, with no trailing slash
// (a provider answers ".../chat/completions/" with 404).
func TestNewReverseProxy_EmptyIncomingPathReachesTheTargetEndpointExactly(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gotPath = r.URL.Path }))
	defer upstream.Close()

	tests := []struct{ name, incoming, want string }{
		{"empty path uses the target endpoint as is", "", "/provider/v1/chat/completions"},
		{"a real path is still joined to the target", "/v1/models", "/provider/v1/chat/completions/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.URL.Path = tt.incoming
			NewReverseProxy(upstream.URL+"/provider/v1/chat/completions").ServeHTTP(httptest.NewRecorder(), req)
			if gotPath != tt.want {
				t.Errorf("upstream path = %q, want %q", gotPath, tt.want)
			}
		})
	}
}

// A caller names a model the way /v1/models lists it; a cloud provider only knows its own id for it.
func TestEnsureModelProxyHandler_CloudModelIsSentUnderTheProvidersId(t *testing.T) {
	var gotPath, gotModel string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		gotModel, _ = gotBody["model"].(string)
	}))
	defer upstream.Close()

	rt := &fakeRuntime{instance: llm.ModelInstance{Name: "flash", ModelID: "vendor/flash", URL: upstream.URL + "/provider/v1/chat/completions"}}
	body := `{"model":"flash","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	NewProxyHandlers(rt).EnsureModelProxyHandler(httptest.NewRecorder(), req)

	if gotModel != "vendor/flash" {
		t.Errorf("upstream model = %q, want the provider id vendor/flash", gotModel)
	}
	if gotPath != "/provider/v1/chat/completions" {
		t.Errorf("upstream path = %q, want the target endpoint exactly", gotPath)
	}
	if gotBody["max_tokens"] != float64(5) || gotBody["messages"] == nil {
		t.Errorf("the rest of the request was not forwarded intact: %v", gotBody)
	}
}

type countingSink struct {
	mu    sync.Mutex
	calls []modelstats.Call
}

func (c *countingSink) Record(call modelstats.Call) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

// flushWriter is a response writer that can be flushed, as a real one can: a streamed answer depends on it.
type flushWriter struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushWriter) Flush() { f.flushed = true; f.ResponseRecorder.Flush() }

// A call an outside program makes to /v1 is counted with the model, whether the model is local or in the cloud, and whether the answer was an error; the
// answer itself still reaches the caller, a flush still passes through to it, and the tokens are not read (the handler streams the answer untouched).
func TestEnsureModelProxyHandler_CountsTheCallAndStillStreams(t *testing.T) {
	restore := SetReverseProxyFactory(func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Fail") != "" {
				http.Error(w, "boom", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte("data: hi\n\n"))
			w.(http.Flusher).Flush()
		})
	})
	defer restore()
	sink := &countingSink{}
	for _, tc := range []struct {
		name       string
		inst       llm.ModelInstance
		fail       bool
		wantLocal  bool
		wantFailed bool
	}{
		{"local ok", llm.ModelInstance{Name: "qwen", Host: "127.0.0.1", Port: 8080}, false, true, false},
		{"cloud ok", llm.ModelInstance{Name: "flash", ModelID: "vendor/flash", URL: "https://cloud.example/v1/chat/completions"}, false, false, false},
		{"cloud error", llm.ModelInstance{Name: "flash", ModelID: "vendor/flash", URL: "https://cloud.example/v1/chat/completions"}, true, false, true},
	} {
		h := NewProxyHandlers(&fakeRuntime{instance: tc.inst})
		h.SetUsageCounter(sink)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+tc.inst.Name+`","messages":[]}`))
		if tc.fail {
			req.Header.Set("X-Fail", "1")
		}
		w := &flushWriter{ResponseRecorder: httptest.NewRecorder()}
		h.EnsureModelProxyHandler(w, req)
		got := sink.calls[len(sink.calls)-1]
		if got.Model != tc.inst.Name || got.Local != tc.wantLocal || got.Failed != tc.wantFailed || got.PromptTokens != 0 {
			t.Errorf("%s: counted %+v", tc.name, got)
		}
		if !tc.fail && (!w.flushed || !strings.Contains(w.Body.String(), "data: hi")) {
			t.Errorf("%s: the stream did not reach the caller (flushed %v, body %q)", tc.name, w.flushed, w.Body)
		}
	}
	if len(sink.calls) != 3 {
		t.Errorf("counted %d calls, want 3", len(sink.calls))
	}
}

// A call that is refused before it reaches a model (no model could be ensured) is not a model call and is not counted.
func TestEnsureModelProxyHandler_DoesNotCountACallThatNeverReachedAModel(t *testing.T) {
	sink := &countingSink{}
	h := NewProxyHandlers(&fakeRuntime{ensureErr: errors.New("no such model")})
	h.SetUsageCounter(sink)
	h.EnsureModelProxyHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`)))
	if len(sink.calls) != 0 {
		t.Errorf("counted %+v", sink.calls)
	}
}

// A stream the proxy drops half way (ReverseProxy panics with http.ErrAbortHandler to close the connection) is still one call.
func TestEnsureModelProxyHandler_CountsAStreamThatWasAbortedHalfWay(t *testing.T) {
	restore := SetReverseProxyFactory(func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) })
	})
	defer restore()
	sink := &countingSink{}
	h := NewProxyHandlers(&fakeRuntime{instance: llm.ModelInstance{Name: "qwen", Host: "127.0.0.1", Port: 8080}})
	h.SetUsageCounter(sink)
	func() {
		defer func() { _ = recover() }()
		h.EnsureModelProxyHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen"}`)))
	}()
	if len(sink.calls) != 1 || sink.calls[0].Model != "qwen" {
		t.Errorf("counted %+v", sink.calls)
	}
}
