package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/llm"
	"llm-proxy/models"
)

// fakeGate is a scripted InboundGate: it answers with err (or admits), and can
// block until its ctx ends to simulate a caller waiting for the model.
type fakeGate struct {
	maxWait     int
	err         error
	release     func()
	block       bool
	waitCalls   int
	lastReq     InboundRequest
	cancelled   string
	cancelFound bool
}

func (g *fakeGate) Cancel(key string) bool {
	g.cancelled = key
	return g.cancelFound
}

func (g *fakeGate) WaitSeconds(requested int, present bool) int {
	if !present {
		return g.maxWait
	}
	if g.maxWait == 0 || requested <= 0 {
		return 0
	}
	if g.maxWait < 0 || requested < g.maxWait {
		return requested
	}
	return g.maxWait
}

func (g *fakeGate) Wait(ctx context.Context, req InboundRequest) (func(), error) {
	g.waitCalls++
	g.lastReq = req
	if g.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if g.err != nil {
		return nil, g.err
	}
	return g.release, nil
}

func inboundRequest(model, queueWait string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"`+model+`","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	if queueWait != "" {
		req.Header.Set(queueWaitHeader, queueWait)
	}
	return req
}

func localModel(name string) models.ModelConfig {
	return models.ModelConfig{Name: name, Provider: models.ProviderLocal, Filename: name + ".gguf"}
}

// liveUpstream starts an upstream a fake RuntimeService can point at, so the
// admitted path completes through the reverse proxy instead of dialing nothing.
func liveUpstream(t *testing.T) (local llm.ModelInstance, endpoint string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"served":true}`))
	}))
	t.Cleanup(srv.Close)

	u, err := neturl.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("test server port: %v", err)
	}
	return llm.ModelInstance{Host: u.Hostname(), Port: port}, srv.URL
}

func TestInbound_RefusedWhenTheCallerWillNotWait(t *testing.T) {
	rt := &fakeRuntime{
		models:     []models.ModelConfig{localModel("a"), localModel("b")},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
	}
	h := NewProxyHandlers(rt)
	h.SetInboundGate(&fakeGate{maxWait: 60, err: ErrInboundBusy})

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", "0"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-LLM-Status"); got != models.ModelStatusBusy {
		t.Fatalf("X-LLM-Status = %q, want %q", got, models.ModelStatusBusy)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("a refused caller needs Retry-After to know it may come back")
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(rec.Body.String(), "a") {
		t.Fatalf("body should name the model being served, got %s", rec.Body.String())
	}
	// The whole point: the running model is never evicted on a refusal.
	if rt.ensureCalls != 0 {
		t.Fatalf("EnsureModel called %d times, want 0 (must not evict)", rt.ensureCalls)
	}
}

// A caller that sends no X-Queue-Wait header gets whatever budget the gate's
// park policy resolves: when it resolves a hold, the caller is parked and
// answered "queued" when the wait is spent, instead of being refused.
func TestInbound_AbsentHeaderIsHeldWhenTheGateParksIt(t *testing.T) {
	rt := &fakeRuntime{
		models:     []models.ModelConfig{localModel("a"), localModel("b")},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
	}
	h := NewProxyHandlers(rt)
	h.SetInboundGate(&fakeGate{maxWait: 1, block: true})

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", ""))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the park budget is spent: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-LLM-Status"); got != models.ModelStatusQueued {
		t.Fatalf("X-LLM-Status = %q, want %q", got, models.ModelStatusQueued)
	}
	if body := rec.Body.String(); !strings.Contains(body, "waited as long as allowed") {
		t.Fatalf("a parked caller's expired answer must say it waited, got %s", body)
	}
	if rt.ensureCalls != 0 {
		t.Fatalf("EnsureModel called %d times, want 0 (never served)", rt.ensureCalls)
	}
}

func TestInbound_AdmittedCallerProceedsAndReleases(t *testing.T) {
	instance, _ := liveUpstream(t)
	released := false
	rt := &fakeRuntime{
		models:     []models.ModelConfig{localModel("a"), localModel("b")},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
		instance:   instance,
	}
	h := NewProxyHandlers(rt)
	h.SetInboundGate(&fakeGate{maxWait: 60, release: func() { released = true }})

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", "30"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 once admitted: %s", rec.Code, rec.Body.String())
	}
	if rt.ensureCalls != 1 {
		t.Fatalf("EnsureModel called %d times, want 1 once admitted", rt.ensureCalls)
	}
	if !released {
		t.Fatal("the admission claim must be released when the request finishes")
	}
}

func TestInbound_BudgetExpiryAnswersQueued(t *testing.T) {
	rt := &fakeRuntime{
		models:     []models.ModelConfig{localModel("a"), localModel("b")},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
	}
	h := NewProxyHandlers(rt)
	h.SetInboundGate(&fakeGate{maxWait: 60, block: true})

	rec := httptest.NewRecorder()
	start := time.Now()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", "1"))

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v, want the caller's 1s budget honoured", elapsed)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-LLM-Status"); got != models.ModelStatusQueued {
		t.Fatalf("X-LLM-Status = %q, want %q", got, models.ModelStatusQueued)
	}
	if rt.ensureCalls != 0 {
		t.Fatalf("EnsureModel called %d times, want 0 (never served)", rt.ensureCalls)
	}
}

func TestInbound_CloudModelBypassesTheGate(t *testing.T) {
	_, endpoint := liveUpstream(t)
	rt := &fakeRuntime{
		models:     []models.ModelConfig{{Name: "gpt", Provider: "openai"}},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
		instance:   llm.ModelInstance{URL: endpoint},
	}
	h := NewProxyHandlers(rt)
	gate := &fakeGate{maxWait: 60, err: ErrInboundBusy}
	h.SetInboundGate(gate)

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("gpt", ""))

	if gate.waitCalls != 0 {
		t.Fatal("a provider model never touches the local slot: the gate must not be consulted")
	}
	if rt.ensureCalls != 1 {
		t.Fatalf("EnsureModel called %d times, want 1", rt.ensureCalls)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestInbound_BusyFromTheModelManagerAnswersBusy(t *testing.T) {
	// A run admitted after the gate's approval owns the model by the time
	// EnsureModel runs: the caller gets a retry answer, not a 500.
	rt := &fakeRuntime{
		models:     []models.ModelConfig{localModel("b")},
		activeInfo: &llm.ActiveModelInfo{Name: "a"},
		ensureErr:  llm.ErrLocalModelBusy,
	}
	h := NewProxyHandlers(rt)

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", ""))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-LLM-Status"); got != models.ModelStatusBusy {
		t.Fatalf("X-LLM-Status = %q, want %q", got, models.ModelStatusBusy)
	}
}

func TestInbound_WithoutAGateRequestsAreUnchanged(t *testing.T) {
	instance, _ := liveUpstream(t)
	rt := &fakeRuntime{models: []models.ModelConfig{localModel("b")}, instance: instance}
	h := NewProxyHandlers(rt)

	rec := httptest.NewRecorder()
	h.EnsureModelProxyHandler(rec, inboundRequest("b", ""))

	if rt.ensureCalls != 1 {
		t.Fatalf("EnsureModel called %d times, want 1", rt.ensureCalls)
	}
	if got := rec.Header().Get("X-LLM-Status"); got != "" {
		t.Fatalf("ungated request must carry no inbound status, got %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestParseQueueWait(t *testing.T) {
	cases := map[string]struct {
		want    int
		present bool
	}{
		"":     {0, false},
		"0":    {0, true},
		"30":   {30, true},
		"-5":   {0, true},
		"abc":  {0, true},
		" 12 ": {12, true},
	}
	for raw, want := range cases {
		got, present := parseQueueWait(raw)
		if got != want.want || present != want.present {
			t.Fatalf("parseQueueWait(%q) = (%d, %v), want (%d, %v)", raw, got, present, want.want, want.present)
		}
	}
}

// A caller's queue key is its cancel handle on the public /v1 surface, so it
// must not be guessable from another caller's.
func TestInboundKeysAreUnguessableAndDistinct(t *testing.T) {
	h := NewProxyHandlers(&fakeRuntime{})

	seen := make(map[string]bool, 64)
	for range 64 {
		key := h.nextInboundKey()
		if !strings.HasPrefix(key, inboundKeyPrefix) {
			t.Fatalf("key %q lost the %q prefix the frontend keys on", key, inboundKeyPrefix)
		}
		if len(key) <= len(inboundKeyPrefix)+8 {
			t.Fatalf("key %q carries no meaningful random part", key)
		}
		if seen[key] {
			t.Fatalf("duplicate key %q", key)
		}
		seen[key] = true
	}
}

func TestInbound_ServeQueueCancelMatchesAKey(t *testing.T) {
	gate := &fakeGate{cancelFound: true}
	h := NewProxyHandlers(&fakeRuntime{})
	h.SetInboundGate(gate)

	req := httptest.NewRequest(http.MethodDelete, "/v1/queue/inbound:x", nil)
	req.SetPathValue(QueueKeyParam, "inbound:x")
	rec := httptest.NewRecorder()
	h.ServeQueueCancel(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if gate.cancelled != "inbound:x" {
		t.Fatalf("cancelled %q, want the path key", gate.cancelled)
	}
}

func TestInbound_ServeQueueCancelReportsAnUnknownKey(t *testing.T) {
	h := NewProxyHandlers(&fakeRuntime{})
	h.SetInboundGate(&fakeGate{cancelFound: false})

	req := httptest.NewRequest(http.MethodDelete, "/v1/queue/inbound:gone", nil)
	req.SetPathValue(QueueKeyParam, "inbound:gone")
	rec := httptest.NewRecorder()
	h.ServeQueueCancel(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (served, expired, or cancelled already)", rec.Code)
	}
}

func TestInbound_ServeQueueCancelUnavailableWithoutAGate(t *testing.T) {
	h := NewProxyHandlers(&fakeRuntime{})

	req := httptest.NewRequest(http.MethodDelete, "/v1/queue/inbound:x", nil)
	req.SetPathValue(QueueKeyParam, "inbound:x")
	rec := httptest.NewRecorder()
	h.ServeQueueCancel(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when no gate is wired", rec.Code)
	}
}
