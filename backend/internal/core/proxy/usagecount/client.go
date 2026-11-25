// Package usagecount wraps a model client so every finished call is counted (ADR-0085). It reads only what the response itself carries: the token counts the
// provider reported and the speed a local server reported. It never reads or keeps a prompt or a reply, and it changes nothing the caller sees: every chunk
// is passed on as it arrived and every error is returned as it was.
package usagecount

import (
	"context"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/modelstats"
)

// Sink takes one counted call; modelstats.Store is the one in the service.
type Sink interface {
	Record(modelstats.Call)
}

// Client counts the calls of the client it wraps.
type Client struct {
	inner proxy.Client
	sink  Sink
	model string
	local bool
}

// New wraps inner. model names the calls that carry no model of their own; whether the model is local is asked of inner once, here.
func New(inner proxy.Client, sink Sink, model string) *Client {
	return &Client{inner: inner, sink: sink, model: model, local: proxy.IsLocalClient(inner)}
}

func (c *Client) call(req proxy.ChatRequest) modelstats.Call {
	m := c.model
	if req.Model != "" {
		m = req.Model
	}
	return modelstats.Call{Model: m, Local: c.local}
}

// ReasoningField is the wrapped client's: the agent asks the real upstream which wire field the reasoning budget uses.
func (c *Client) ReasoningField() string { return c.inner.ReasoningField() }

// IsLocal is the wrapped client's, so a local-only run sees the real upstream and not this wrapper.
func (c *Client) IsLocal() bool { return c.local }

// Chat makes the call and counts it.
func (c *Client) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	resp, err := c.inner.Chat(ctx, req)
	rec := c.call(req)
	if err != nil {
		rec.Failed = true
	} else if resp != nil {
		fill(&rec, resp)
	}
	c.sink.Record(rec)
	return resp, err
}

// Stream starts the call and counts it when the stream ends, whether it ended by itself or because the caller stopped reading. Chunks are passed on
// untouched; the count is taken from the last usage and the last speed any chunk carried.
func (c *Client) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	inner, err := c.inner.Stream(ctx, req)
	rec := c.call(req)
	if err != nil {
		rec.Failed = true
		c.sink.Record(rec)
		return nil, err
	}
	out := make(chan *proxy.ChatResponse, cap(inner))
	go func() {
		defer close(out)
		defer func() { c.sink.Record(rec) }()
		for chunk := range inner {
			if chunk != nil {
				fill(&rec, chunk)
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// fill takes the tokens and the speed a response carries, keeping what an earlier chunk gave when this one says nothing.
func fill(rec *modelstats.Call, r *proxy.ChatResponse) {
	if u := r.ReportedUsage(); u != nil {
		rec.PromptTokens, rec.CompletionTokens = u.PromptTokens, u.CompletionTokens
	}
	if r.Timings != nil && r.Timings.PredictedPerSecond > 0 {
		rec.TokensPerSec = r.Timings.PredictedPerSecond
	}
}
