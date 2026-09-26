package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

// Inbound admission for external /v1 callers.
//
// The local slot serves one model at a time, so serving a different model means
// stopping the running server — which kills any run using it. An external caller
// therefore never gets to evict: it waits within a budget, or is told to retry.
// The gate (the run scheduler's model-residency gate, injected at the
// composition root) decides; this file only spells the wire behaviour.
const (
	// queueWaitHeader is the caller's declared patience, in seconds. Absent
	// means "the host decides": the caller is parked up to InboundWaitSeconds
	// only when the host enabled inbound_wait_by_default, and refused
	// otherwise. An explicit 0 means "do not wait" — refused rather than held.
	queueWaitHeader = "X-Queue-Wait"
	// retryAfterSeconds tells a refused caller when to come back. A guess is
	// honest here: nothing can predict when the running work ends.
	retryAfterSeconds = 5
	// inboundKeyPrefix names a waiting caller's minted identity: its cancel
	// handle, its row key in the operator UI, and what the frontend uses to tell
	// an external caller from a lane-queued run.
	inboundKeyPrefix = "inbound:"
)

var (
	// ErrInboundBusy reports that the local model is in use and policy does not
	// let this caller wait.
	ErrInboundBusy = errors.New("local model is in use")
	// ErrInboundQueueFull reports that too many callers are already waiting.
	ErrInboundQueueFull = errors.New("inbound wait queue is full")
)

// InboundRequest is one external caller asking for the local model.
type InboundRequest struct {
	Model  string // model the caller wants served
	Active string // local model currently served ("" when none)
	Key    string // stable identity: cancel/dismiss handle and UI row key
	Label  string // human label for the UI
	// Wait is the caller's effective budget in seconds, resolved by
	// WaitSeconds. 0 means it must be served now or refused; negative is
	// unbounded. A caller that sent no header gets the host's park policy here
	// (0 when inbound_wait_by_default is off).
	Wait int
}

// InboundGate admits external /v1 callers to the local model. Implemented by the
// composition root over the run scheduler + host settings.
type InboundGate interface {
	// WaitSeconds resolves a caller's effective wait in seconds. A caller that
	// sent an explicit X-Queue-Wait (present=true) is clamped to the host cap;
	// one that sent no header is parked for InboundWaitSeconds only when the
	// host enabled inbound_wait_by_default, and refused otherwise. Either way
	// 0 refuses immediately and -1 waits without a budget.
	WaitSeconds(requested int, present bool) int
	// Wait admits req, blocking while the local model is in use. It returns a
	// release that must be called when the caller has finished using the model,
	// or an error: ErrInboundBusy / ErrInboundQueueFull, ctx.Err() when the
	// budget expired or the client went away, or runlane.ErrModelWaitCancelled
	// when the entry was dropped or the host shut the gate down
	// (runlane.ErrClosed).
	Wait(ctx context.Context, req InboundRequest) (release func(), err error)
	// Cancel drops a waiting caller by its key, reporting whether one matched.
	// A caller may cancel its own entry out of band (the DELETE route) while its
	// request is still held.
	Cancel(key string) bool
}

// parseQueueWait reads the caller's patience from the request header. It
// reports false when the header is absent, so the caller inherits the host
// default; a present but malformed or negative value means "do not wait".
func parseQueueWait(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0, true
	}
	return n, true
}

// isLocalModel reports whether the request targets a manager-launched local
// model — the only models whose serving is exclusive. Cloud/provider models are
// reached over their own endpoint and never evict anything here.
func (h *ProxyHandlers) isLocalModel(name string) bool {
	for _, m := range h.runtime.ListModels() {
		if m.Name == name {
			return m.Provider == "" || m.Provider == models.ProviderLocal
		}
	}
	return false
}

// activeLocalModel names the local model currently served, "" when none.
func (h *ProxyHandlers) activeLocalModel() string {
	info := h.runtime.ActiveInfo()
	if info == nil {
		return ""
	}
	return info.Name
}

// inboundAnswer is one "not served" answer: which HTTP status to answer with,
// what the X-LLM-Status verdict was, and the explanation for the caller.
type inboundAnswer struct {
	httpStatus int
	llmStatus  string
	model      string
	message    string
}

// writeInboundStatus answers a caller whose request was deliberately not served,
// on the same X-LLM-Status + Retry-After channel as the "starting" handshake.
func writeInboundStatus(w http.ResponseWriter, a inboundAnswer) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	w.Header().Set(models.InboundStatusHeader, a.llmStatus)
	w.WriteHeader(a.httpStatus)
	// Best effort: a write failure means the client is already gone, and there
	// is nothing here that could act on it.
	_, _ = w.Write(inboundStatusJSON(a))
}

// inboundStatusBody is the JSON answer for a request that was deliberately not
// served. Marshalled from a struct rather than concatenated so the copy can
// never break the payload.
type inboundStatusBody struct {
	Status        string `json:"status"`
	Model         string `json:"model"`
	Message       string `json:"message"`
	RetryAfterSec int    `json:"retry_after_seconds"`
}

func inboundStatusJSON(a inboundAnswer) []byte {
	// A struct of strings cannot fail to marshal.
	body, _ := json.Marshal(inboundStatusBody{
		Status: a.llmStatus, Model: a.model, Message: a.message, RetryAfterSec: retryAfterSeconds,
	})
	return body
}

// ServeQueueCancel drops a waiting caller by its key (DELETE
// /v1/queue/{queue_key}). It is the out-of-band cancel for a request whose
// connection is held open, and the caller's own handle — the key is returned to
// it in the busy/queued answer. 204 either way it matched, 404 when the entry is
// already gone (served, expired, or cancelled).
func (h *ProxyHandlers) ServeQueueCancel(w http.ResponseWriter, r *http.Request) {
	if h.inbound == nil {
		http.Error(w, "inbound queue is not enabled", http.StatusServiceUnavailable)
		return
	}
	vals, ok := requirePathParams(w, r, QueueKeyParam)
	if !ok {
		return
	}
	if !h.inbound.Cancel(vals[0]) {
		http.Error(w, "no queued caller with that key", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// admitInbound lets an external caller through the residency gate. It reports
// whether the request should proceed; when it writes a response, the caller must
// not continue.
func (h *ProxyHandlers) admitInbound(w http.ResponseWriter, r *http.Request, model string) (func(), bool) {
	if h.inbound == nil || !h.isLocalModel(model) {
		return nil, true
	}

	active := h.activeLocalModel()
	wait := h.inbound.WaitSeconds(parseQueueWait(r.Header.Get(queueWaitHeader)))

	ctx := r.Context()
	if wait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(wait)*time.Second)
		defer cancel()
	}

	release, err := h.inbound.Wait(ctx, InboundRequest{
		Model: model, Active: active, Key: h.nextInboundKey(), Label: model, Wait: wait,
	})
	switch {
	case err == nil:
		return release, true
	case errors.Is(err, ErrInboundBusy), errors.Is(err, ErrInboundQueueFull):
		// Refused, not held. 429 + Retry-After is the standard "busy, back off"
		// answer — OpenAI-compatible clients already retry on it.
		writeInboundStatus(w, inboundAnswer{
			httpStatus: http.StatusTooManyRequests, llmStatus: models.ModelStatusBusy,
			model: model, message: busyMessage(model, active),
		})
		return nil, false
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, runlane.ErrModelWaitCancelled), errors.Is(err, runlane.ErrClosed):
		// The wait ended without the model freeing (or the host is going down):
		// the parked entry was dropped, nothing was accepted for later
		// processing — so this is "retry" (429), not "accepted" (202).
		writeInboundStatus(w, inboundAnswer{
			httpStatus: http.StatusTooManyRequests, llmStatus: models.ModelStatusQueued,
			model: model, message: queuedMessage(model, active),
		})
		return nil, false
	default:
		// The client went away (context.Canceled) or the gate failed closed:
		// nothing useful to write — do not serve.
		return nil, false
	}
}

// busyMessage is deliberately specific about the situation and deliberately
// silent about when it will clear: only the run's own progress knows that.
func busyMessage(model, active string) string {
	if active == "" {
		return "the local model is in use by a running job; " + model + " is not served meanwhile"
	}
	return "the local model is serving " + active + " for a running job; " + model + " would interrupt it"
}

// queuedMessage explains a parked caller whose wait expired without the model
// becoming free. It is deliberately different from busyMessage: the caller did
// wait, so "would interrupt it" would be wrong — nothing was accepted for later
// processing.
func queuedMessage(model, active string) string {
	if active == "" {
		return "the wait for the local model ended before it became available; " + model + " was not served"
	}
	return "waited as long as allowed; the local model is still serving " + active +
		" for a running job, so " + model + " was not served"
}
