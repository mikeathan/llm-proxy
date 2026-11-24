package usagecount

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/modelstats"
	"llm-proxy/models"
)

type sink struct {
	mu    sync.Mutex
	calls []modelstats.Call
	got   chan struct{}
}

func newSink() *sink { return &sink{got: make(chan struct{}, 8)} }

func (s *sink) Record(c modelstats.Call) {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
	s.got <- struct{}{}
}

func (s *sink) wait(t *testing.T) modelstats.Call {
	t.Helper()
	select {
	case <-s.got:
	case <-time.After(2 * time.Second):
		t.Fatal("no call was recorded")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

type fake struct {
	resp   *proxy.ChatResponse
	err    error
	chunks []*proxy.ChatResponse
	local  bool
}

func (f *fake) Chat(context.Context, proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return f.resp, f.err
}

func (f *fake) Stream(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan *proxy.ChatResponse, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (f *fake) ReasoningField() string { return "reasoning_budget" }
func (f *fake) IsLocal() bool          { return f.local }

func usage(p, c int) *proxy.ChatResponse {
	return &proxy.ChatResponse{Usage: &models.TokenUsage{PromptTokens: p, CompletionTokens: c}}
}

func TestChat_CountsTheCallWithItsTokensAndTheConfiguredModel(t *testing.T) {
	s := newSink()
	c := New(&fake{resp: usage(100, 20), local: true}, s, "Qwen3.5")
	if _, err := c.Chat(context.Background(), proxy.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	got := s.wait(t)
	if got.Model != "Qwen3.5" || !got.Local || got.PromptTokens != 100 || got.CompletionTokens != 20 || got.Failed {
		t.Errorf("%+v", got)
	}
}

func TestChat_TheRequestsOwnModelNameWinsAndAFailureIsCounted(t *testing.T) {
	s := newSink()
	c := New(&fake{err: errors.New("boom")}, s, "default")
	if _, err := c.Chat(context.Background(), proxy.ChatRequest{Model: "other"}); err == nil {
		t.Fatal("the error was swallowed")
	}
	got := s.wait(t)
	if got.Model != "other" || got.Local || !got.Failed {
		t.Errorf("%+v", got)
	}
}

func TestStream_PassesEveryChunkOnAndCountsOnceWithTheFinalUsageAndSpeed(t *testing.T) {
	s := newSink()
	final := &proxy.ChatResponse{Usage: &models.TokenUsage{PromptTokens: 50, CompletionTokens: 9}, Timings: &models.Timings{PredictedPerSecond: 42.5}}
	f := &fake{chunks: []*proxy.ChatResponse{{}, {}, final}, local: true}
	ch, err := New(f, s, "m").Stream(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for range ch {
		n++
	}
	if n != 3 {
		t.Errorf("chunks passed on: %d", n)
	}
	got := s.wait(t)
	if got.PromptTokens != 50 || got.CompletionTokens != 9 || got.TokensPerSec != 42.5 {
		t.Errorf("%+v", got)
	}
	if len(s.calls) != 1 {
		t.Errorf("counted %d times", len(s.calls))
	}
}

func TestStream_AFailureToStartIsCountedAndTheErrorReturned(t *testing.T) {
	s := newSink()
	if _, err := New(&fake{err: errors.New("down")}, s, "m").Stream(context.Background(), proxy.ChatRequest{}); err == nil {
		t.Fatal("no error")
	}
	if got := s.wait(t); !got.Failed {
		t.Errorf("%+v", got)
	}
}

func TestStream_AnAbandonedStreamStillCountsAndLeaksNothing(t *testing.T) {
	s := newSink()
	chunks := make([]*proxy.ChatResponse, 300)
	for i := range chunks {
		chunks[i] = &proxy.ChatResponse{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := New(&fake{chunks: chunks}, s, "m").Stream(ctx, proxy.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	<-ch
	cancel()
	s.wait(t)
}

func TestClient_SaysWhatTheClientUnderneathSaysAboutLocalityAndReasoning(t *testing.T) {
	c := New(&fake{local: true}, newSink(), "m")
	if !proxy.IsLocalClient(c) || c.ReasoningField() != "reasoning_budget" {
		t.Error("the wrapper hides what the real client says")
	}
	if proxy.IsLocalClient(New(&fake{}, newSink(), "m")) {
		t.Error("a cloud client reads as local")
	}
}
