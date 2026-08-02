package models

import (
	"context"
	"errors"
	"net/http"
)

var (
	ErrModelStarting = errors.New("model is starting")
	ErrUnknownModel  = errors.New("unknown model")
	ErrModelExists   = errors.New("model already exists")
)

type ProviderStatus string

const (
	ProviderStatusRunning ProviderStatus = "running"
	ProviderStatusReady   ProviderStatus = "ready"
	ProviderStatusError   ProviderStatus = "error"
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
	ContextLength  int `json:"context_length,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type Provider interface {
	Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	GetStatus() ProviderStatus
	Shutdown() error

	EnsureReady(ctx context.Context) error
	GetEndpoint(ctx context.Context) (string, http.Header, error)
	ListModels(ctx context.Context) ([]ProviderModelInfo, error)
	TestConnection(ctx context.Context) error
}
