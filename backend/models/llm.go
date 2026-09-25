package models

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ModelStartPollInterval is how often a caller re-checks whether a model has
// finished starting. Shared by the automation executor's local cold-start wait
// and the proxy client's upstream 202 "starting" poll so both layers agree on
// the cadence. A var (not const) so tests can shorten it.
var ModelStartPollInterval = 3 * time.Second

// ModelStatusStarting is the status value the proxy returns (HTTP 202 with a
// {"status":"starting"} body) while a model is still loading, and the value
// the proxy client recognizes to poll until the model is ready. One constant
// at both ends keeps the producer and consumer from drifting apart.
const ModelStatusStarting = "starting"

// Inbound-admission statuses, carried on X-LLM-Status like ModelStatusStarting.
// The local slot serves one model at a time, so a request that would evict a
// model a run is using is answered instead of served:
//
//   - ModelStatusBusy: refused right now — the caller may retry (Retry-After).
//   - ModelStatusQueued: the caller was parked, but the wait ended unserved —
//     nothing was accepted for later processing, so the caller retries or gives up.
//
// Both answers go out as HTTP 429 + Retry-After: the standard "busy, back off"
// signal, which OpenAI-compatible clients already retry on. (409/202 were used
// before 2026-09-25; this proxy's client still recognizes them for
// mixed-version deployments.)
const (
	ModelStatusBusy   = "busy"
	ModelStatusQueued = "queued"
)

var (
	ErrModelStarting = errors.New("model is starting")
	ErrUnknownModel  = errors.New("unknown model")
	ErrModelExists   = errors.New("model already exists")
)

type ModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type ModelLimits struct {
	Context int `json:"context,omitempty"`
}

type ModelMeta struct {
	ContextLength int   `json:"n_ctx_train,omitempty"`
	Nctx          int   `json:"n_ctx,omitempty"`
	Parameters    int64 `json:"n_params,omitempty"`
}

type ProviderModelInfo struct {
	ID      string        `json:"id"`
	Pricing *ModelPricing `json:"pricing,omitempty"`
	Limits  *ModelLimits  `json:"limits,omitempty"`
	Meta    *ModelMeta    `json:"meta,omitempty"`

	// Published capabilities extracted from the provider's live catalog
	// (§2.10): context window and per-model output cap.  Explicit fields — we
	// do not overload the llama-specific ModelMeta names.
	ContextLength   int `json:"context_length,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type Provider interface {
	EnsureReady(ctx context.Context) error
	GetEndpoint(ctx context.Context) (string, http.Header, error)
	ListModels(ctx context.Context) ([]ProviderModelInfo, error)
	TestConnection(ctx context.Context) error
}
