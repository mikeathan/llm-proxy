package models

import (
	"context"
	"encoding/json"
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

// InboundStatusHeader carries the inbound-admission verdict (a ModelStatus*
// value) alongside the HTTP status. One home for the header name keeps the
// producer (proxy handler) and consumers (proxy client) from drifting apart.
const InboundStatusHeader = "X-LLM-Status"

// IsResidencyStatus reports whether s is one of the inbound-admission residency
// verdicts (busy/queued). Shared by the proxy client (which reads the
// X-LLM-Status header) and the failure classifier (which reads the body) so the
// producer and every consumer agree on the contract.
func IsResidencyStatus(s string) bool {
	return s == ModelStatusBusy || s == ModelStatusQueued
}

// ResidencyAnswer parses a proxy's residency answer body — the
// {"status":"busy"|"queued","message":"..."} shape the inbound gate emits —
// returning the verdict and its human explanation. It returns ("", "") for any
// other body, so callers need no separate JSON handling.
func ResidencyAnswer(body string) (status, message string) {
	var probe struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &probe) != nil || !IsResidencyStatus(probe.Status) {
		return "", ""
	}
	return probe.Status, probe.Message
}

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
