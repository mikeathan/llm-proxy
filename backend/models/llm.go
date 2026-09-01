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
