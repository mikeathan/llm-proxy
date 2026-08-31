package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/models"
)

// TestExtractCapability_ProviderShapes is V5: one extractCapability parses the
// published shapes across providers.
func TestExtractCapability_ProviderShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		keys    []string
		want    int
	}{
		{
			name:    "openrouter top_provider.max_completion_tokens",
			payload: `{"id":"deepseek/deepseek-v4-flash","top_provider":{"max_completion_tokens":384000}}`,
			keys:    OutputCapKeys,
			want:    384000,
		},
		{
			name:    "openrouter top_provider.context_length",
			payload: `{"id":"m","top_provider":{"context_length":1048576}}`,
			keys:    ContextLengthKeys,
			want:    1048576,
		},
		{
			name:    "openrouter top-level context_length",
			payload: `{"id":"m","context_length":131072}`,
			keys:    ContextLengthKeys,
			want:    131072,
		},
		{
			name:    "openai max_completion_tokens",
			payload: `{"id":"gpt-4o","max_completion_tokens":16384}`,
			keys:    OutputCapKeys,
			want:    16384,
		},
		{
			name:    "nvidia limits.context",
			payload: `{"id":"m","limits":{"context":128000}}`,
			keys:    ContextLengthKeys,
			want:    128000,
		},
		{
			name:    "llamacpp meta.n_ctx",
			payload: `{"id":"qwen.gguf","meta":{"n_ctx_train":262144,"n_ctx":8192}}`,
			keys:    ContextLengthKeys,
			want:    8192,
		},
		{
			name:    "lm studio loaded_instances config.context_length",
			payload: `{"data":[{"id":"m","loaded_instances":[{"config":{"context_length":32768}}]}]}`,
			keys:    ContextLengthKeys,
			want:    32768,
		},
		{
			name:    "nested array slice",
			payload: `{"a":[{"b":{"max_tokens":"4096"}}]}`,
			keys:    OutputCapKeys,
			want:    4096,
		},
		{
			name:    "no match",
			payload: `{"id":"m"}`,
			keys:    OutputCapKeys,
			want:    0,
		},
		{
			name:    "absurd value ignored",
			payload: `{"max_tokens":9999999999}`,
			keys:    OutputCapKeys,
			want:    0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var node any
			if err := json.Unmarshal([]byte(c.payload), &node); err != nil {
				t.Fatal(err)
			}
			if got := extractCapability(node, c.keys); got != c.want {
				t.Fatalf("extractCapability = %d, want %d", got, c.want)
			}
		})
	}
}

// TestListModels_OpenRouterRealPayload verifies §5: the real OpenRouter shape
// parses (context_length + top_provider.max_completion_tokens) and /slots is
// NOT probed for a cloud URL.
func TestListModels_OpenRouterRealPayload(t *testing.T) {
	slotsHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" || r.URL.Path == "/v1/slots" {
			slotsHit = true
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"id":"deepseek/deepseek-v4-flash-0731","context_length":1048576,"top_provider":{"context_length":1048576,"max_completion_tokens":384000},"pricing":{"prompt":"0.00000014","completion":"0.00000028"}},
			{"id":"openai/gpt-4o-2024-05-13","context_length":128000,"max_completion_tokens":4096}
		]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openrouter")
	cfg := models.ModelConfig{
		Provider: "openrouter",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})
	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slotsHit {
		t.Fatal("cloud listing must not probe /slots")
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 models, got %d", len(infos))
	}
	if infos[0].ID != "deepseek/deepseek-v4-flash-0731" {
		t.Errorf("unexpected id: %s", infos[0].ID)
	}
	if infos[0].MaxOutputTokens != 384000 {
		t.Errorf("expected deepseek output cap 384000, got %d", infos[0].MaxOutputTokens)
	}
	if infos[0].ContextLength != 1048576 {
		t.Errorf("expected deepseek context 1048576, got %d", infos[0].ContextLength)
	}
	if infos[0].Pricing == nil || infos[0].Pricing.Prompt != "0.00000014" {
		t.Errorf("expected pricing parsed, got %+v", infos[0].Pricing)
	}
	if infos[1].MaxOutputTokens != 4096 {
		t.Errorf("expected gpt-4o output cap 4096, got %d", infos[1].MaxOutputTokens)
	}
}

// TestListModels_LocalSlotsGated verifies §3.4: a local endpoint probes /slots
// and the recovered n_ctx reaches the published context.
func TestListModels_LocalSlotsGated(t *testing.T) {
	var slotsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" {
			slotsHits++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"n_ctx":8192,"is_processing":false}]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"llama-alias"}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	cfg := models.ModelConfig{
		Provider: "openai",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: server.URL,
		},
	}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})
	p.SetWorkloadClassifier(models.NewWorkloadClassifier("127.0.0.1", nil))
	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slotsHits == 0 {
		t.Fatal("local endpoint must probe /slots")
	}
	if len(infos) != 1 || infos[0].ContextLength != 8192 {
		t.Fatalf("expected /slots n_ctx 8192 on the model, got %+v", infos)
	}
}

// TestListModels_RemoteLlamaCPPServingContext is the production-fix regression
// test: a REMOTE llama.cpp host serving a GGUF model (owned_by "llamacpp",
// meta.n_ctx_train 262144) is NOT effective-local (no classifier), yet the
// /slots probe MUST still run because the listing itself carries the llama.cpp
// fingerprint — and the SERVING n_ctx (8192) must OVERRIDE the training-derived
// n_ctx_train (262144) so the local budget keys on the real serving window,
// never the training context (SPEC-005 priority 1).
func TestListModels_RemoteLlamaCPPServingContext(t *testing.T) {
	var slotsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" {
			slotsHits++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"n_ctx":8192,"is_processing":false}]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"Qwen3.6-35B-A3B","owned_by":"llamacpp","meta":{"n_ctx_train":262144,"n_params":20914757184}}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})
	// No classifier injected: the listing fingerprint alone must trigger the probe.
	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slotsHits == 0 {
		t.Fatal("remote llama.cpp listing must probe /slots (listing fingerprint, not host locality)")
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 model, got %d", len(infos))
	}
	if infos[0].ContextLength != 8192 {
		t.Fatalf("serving n_ctx must override n_ctx_train: got ContextLength %d, want 8192", infos[0].ContextLength)
	}
	if infos[0].Meta == nil || infos[0].Meta.Nctx != 8192 {
		t.Fatalf("probe must carry the serving n_ctx on Meta.Nctx for discovery: got %+v", infos[0].Meta)
	}
}

// TestListModels_SlotsOverridesTrainingContext verifies the precedence half of
// the fix: even when the endpoint is effective-local and the /v1/models listing
// carries n_ctx_train (262144), the /slots probe result (8192) must WIN — the
// probe previously only filled an EMPTY ContextLength, so a training-derived
// value would have survived and inflated the local budget.
func TestListModels_SlotsOverridesTrainingContext(t *testing.T) {
	var slotsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" {
			slotsHits++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"n_ctx":8192,"is_processing":false}]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"qwen-alias","meta":{"n_ctx_train":262144}}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})
	p.SetWorkloadClassifier(models.NewWorkloadClassifier("127.0.0.1", nil))
	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slotsHits == 0 {
		t.Fatal("effective-local endpoint must probe /slots")
	}
	if len(infos) != 1 || infos[0].ContextLength != 8192 {
		t.Fatalf("serving n_ctx must override training n_ctx_train, got %+v", infos)
	}
}

// TestListModels_SlotsTimeout verifies S2: a dead local server does not hang
// the listing — the /slots probe exits within its child deadline.
func TestListModels_SlotsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" || r.URL.Path == "/v1/slots" {
			time.Sleep(2 * time.Second)
			w.Write([]byte("late"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model1"}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})
	p.SetWorkloadClassifier(models.NewWorkloadClassifier("127.0.0.1", nil))

	start := time.Now()
	infos, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > slotsProbeTimeout+500*time.Millisecond {
		t.Fatalf("listing hung on slow /slots probe: %v", elapsed)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 model, got %d", len(infos))
	}
}

// TestListModels_CatalogCached verifies V5: the catalog is fetched once and
// served from the TTL cache on subsequent calls.
func TestListModels_CatalogCached(t *testing.T) {
	var fetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model1"},{"id":"model2"}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
	p := NewOpenAICompatibleProviderWithDoer(cfg, m, &http.Client{Transport: server.Client().Transport})

	for i := 0; i < 3; i++ {
		infos, err := p.ListModels(context.Background())
		if err != nil {
			t.Fatalf("ListModels: %v", err)
		}
		if len(infos) != 2 {
			t.Fatalf("expected 2 models, got %d", len(infos))
		}
	}
	if fetches != 1 {
		t.Fatalf("catalog must be fetched once and cached, got %d fetches", fetches)
	}
}

// TestListModels_SharedCatalogCacheAcrossInstances verifies the catalog cache
// is SHARED across separately-built provider instances (the registrar injects
// one cache, because providers are built fresh per call — a per-instance cache
// would refetch the catalog on every discovery request, V5).
func TestListModels_SharedCatalogCacheAcrossInstances(t *testing.T) {
	var fetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model1"},{"id":"model2"}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	doer := &http.Client{Transport: server.Client().Transport}
	shared := core.NewTTLCache[string, []models.ProviderModelInfo](catalogCacheMaxEntries, catalogTTL, nil)

	// Two independent providers — as built by the registrar on separate calls —
	// must share the catalog cache so the second call never refetches.
	cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
	p1 := NewOpenAICompatibleProviderWithDoer(cfg, m, doer)
	p1.SetCatalogCache(shared)
	if _, err := p1.ListModels(context.Background()); err != nil {
		t.Fatalf("p1.ListModels: %v", err)
	}

	p2 := NewOpenAICompatibleProviderWithDoer(cfg, m, doer)
	p2.SetCatalogCache(shared)
	if _, err := p2.ListModels(context.Background()); err != nil {
		t.Fatalf("p2.ListModels: %v", err)
	}

	if fetches != 1 {
		t.Fatalf("shared catalog must be fetched once across instances, got %d fetches", fetches)
	}
}

// TestListModels_ConcurrentCatalogCache verifies concurrent ListModels calls on
// separately-built providers sharing one cache do not race, serve the same
// listing, and share a SINGLE catalog fetch on a cold cache (Get is
// single-flight — run under -race).
func TestListModels_ConcurrentCatalogCache(t *testing.T) {
	var fetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model1"},{"id":"model2"}]}`))
	}))
	defer server.Close()

	m, _ := GetRegistry().Get("openai")
	doer := &http.Client{Transport: server.Client().Transport}
	shared := core.NewTTLCache[string, []models.ProviderModelInfo](catalogCacheMaxEntries, catalogTTL, nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := models.ModelConfig{Provider: "openai", ProviderConfig: &models.ProviderConfig{BaseURL: server.URL}}
			p := NewOpenAICompatibleProviderWithDoer(cfg, m, doer)
			p.SetCatalogCache(shared)
			infos, err := p.ListModels(context.Background())
			if err != nil {
				t.Errorf("ListModels: %v", err)
				return
			}
			if len(infos) != 2 {
				t.Errorf("expected 2 models, got %d", len(infos))
			}
		}()
	}
	wg.Wait()

	if fetches != 1 {
		t.Fatalf("concurrent cold-cache calls must share one fetch (single-flight), got %d fetches", fetches)
	}
}

// TestParseOutputCapError verifies §2.6/§5: known provider phrasings convert to
// a typed OutputCapError; unrelated 400s do not.
func TestParseOutputCapError(t *testing.T) {
	cases := []struct {
		body string
		want *OutputCapError
	}{
		{`{"error":{"message":"max_tokens is greater than the maximum allowed (4096)"}}`, &OutputCapError{Available: 4096}},
		{"max_completion_tokens 8192 exceeds the maximum allowed value of 2048 tokens", &OutputCapError{Available: 2048}},
		{"The maximum allowed value is 16384", &OutputCapError{Available: 16384}},
		// A context-length 400 is NOT an output-cap error: it is a
		// prompt-too-long failure the agent loop's reactive sieve handles
		// (isContextSizeError). Matching it here produced the bogus
		// "requested 2730 max_tokens but the model supports at most 8192"
		// misclassification (llama.cpp context-window 400).
		{"maximum context length is 128000 tokens for this model", nil},
		{"context window is 8192 tokens, reduce the prompt length", nil},
		{`{"error":"bad request"}`, nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := ParseOutputCapError(c.body); c.want == nil {
			if got != nil {
				t.Errorf("body %q: expected nil, got %+v", c.body, got)
			}
		} else if got == nil || got.Available != c.want.Available {
			t.Errorf("body %q: expected Available %d, got %+v", c.body, c.want.Available, got)
		}
	}
}

func TestOutputCapError_Message(t *testing.T) {
	e := &OutputCapError{Requested: 8192, Available: 2048}
	if !strings.Contains(e.Error(), "8192") || !strings.Contains(e.Error(), "2048") {
		t.Fatalf("typed error must carry requested/available numbers: %s", e.Error())
	}
	if !errors.Is(e, ErrOutputCapExceeded) {
		t.Fatal("OutputCapError must satisfy ErrOutputCapExceeded via errors.Is")
	}
}
