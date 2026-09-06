// Package failures centralises LLM/agent failure handling for the assistant
// and automation loops. It holds two layers:
//
//   - failure.go — ClassifyRunFailure turns a terminal agent-run error into a
//     short, human-actionable summary + optional next-step hint for the UI
//     (bounded text; the full error stays in the logs).
//   - errors.go — the recovery-routing classifiers (IsPrefillThinkingError,
//     IsContextSizeError, …) the agent loop uses to decide HOW to recover from
//     an upstream/provider error.
//
// Both live here so error logic is not scattered across the assistant package.
package failures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"llm-proxy/internal/core/proxy"
)

// maxFailureBodyLen bounds extracted upstream message text that reaches the UI.
// Full bodies remain in the server logs (callers log err before surfacing).
const maxFailureBodyLen = 300

// FailureInfo is the UI-facing shape of a terminal run failure.
type FailureInfo struct {
	// Error is the short actionable summary shown in the chat (never the raw
	// upstream JSON dump).
	Error string `json:"error"`
	// Hint is an optional "what to do next" line, set only when the failure
	// maps to a known cause. Empty when the cause is unrecognised.
	Hint string `json:"hint,omitempty"`
	// Status is the upstream HTTP status when the failure came from a
	// non-2xx response (0 otherwise).
	Status int `json:"status,omitempty"`
	// Kind classifies the failure: "chat" | "stream" (upstream HTTP), "cancel",
	// "timeout", "transport" (network/server unreachable), or "internal".
	Kind string `json:"kind,omitempty"`
}

const (
	failureKindChat      = "chat"
	failureKindStream    = "stream"
	failureKindCancel    = "cancel"
	failureKindTimeout   = "timeout"
	failureKindTransport = "transport"
	failureKindInternal  = "internal"
)

// ClassifyRunFailure maps any terminal agent error to a FailureInfo. It walks
// the wrapped-error chain (errors.As) for the typed upstream error, then falls
// back to transport/timeout markers. It never panics and always returns a
// bounded, non-empty Error for a non-nil err.
func ClassifyRunFailure(err error) FailureInfo {
	if err == nil {
		return FailureInfo{Error: "", Kind: failureKindInternal}
	}

	if errors.Is(err, context.Canceled) {
		return FailureInfo{Error: "The run was cancelled.", Kind: failureKindCancel}
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		return FailureInfo{
			Error: "The run timed out before completing.",
			Kind:  failureKindTimeout,
			Hint:  "The model server may be busy or still loading; retry the request.",
		}
	}

	var httpErr *proxy.LLMHTTPError
	if errors.As(err, &httpErr) {
		return classifyHTTPFailure(httpErr)
	}

	return classifyGenericFailure(err)
}

// classifyHTTPFailure turns a typed upstream HTTP error into a FailureInfo.
func classifyHTTPFailure(httpErr *proxy.LLMHTTPError) FailureInfo {
	kind := failureKindChat
	if httpErr.Kind == "stream" {
		kind = failureKindStream
	}
	body := httpErr.Body
	lower := strings.ToLower(body)
	summary := fmt.Sprintf("The model server rejected the request (HTTP %d): %s",
		httpErr.StatusCode, extractUpstreamMessage(body))

	switch {
	case strings.Contains(lower, "custom grammar constraints with tools"):
		return FailureInfo{
			Error:  summary,
			Hint:   "The llama.cpp server does not allow a custom grammar together with native tools. This points to an outdated llm-proxy build — rebuild/reinstall the service from the latest code, then retry.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	case isContextOverflowBody(lower):
		return FailureInfo{
			Error:  summary,
			Hint:   "The request exceeds the model's serving context. Check the model's context window (ctx-size / model metadata) or start a fresh conversation.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	case httpErr.StatusCode == 401 || httpErr.StatusCode == 403:
		return FailureInfo{
			Error:  summary,
			Hint:   "The provider rejected the credentials — check the API key for this provider/model.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	case httpErr.StatusCode == 404:
		return FailureInfo{
			Error:  summary,
			Hint:   "The server does not know this model — check the model name/ID in the model settings.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	case httpErr.StatusCode == 429:
		return FailureInfo{
			Error:  summary,
			Hint:   "The upstream is rate-limiting requests — wait a moment and retry.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	case httpErr.StatusCode == 502 || httpErr.StatusCode == 503 || httpErr.StatusCode == 504 || httpErr.StatusCode == 529:
		return FailureInfo{
			Error:  summary,
			Hint:   "The upstream is temporarily unavailable — retry in a moment.",
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	default:
		return FailureInfo{
			Error:  summary,
			Status: httpErr.StatusCode,
			Kind:   kind,
		}
	}
}

// classifyGenericFailure handles non-HTTP failures (network, server down,
// connection resets) plus anything unrecognised, always bounded.
func classifyGenericFailure(err error) FailureInfo {
	text := err.Error()
	lower := strings.ToLower(text)

	switch {
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "connectex"):
		return FailureInfo{
			Error: fmt.Sprintf("Could not reach the model server: %s", bound(text, maxFailureBodyLen)),
			Hint:  "Check that the model server (llama.cpp / provider) is running and its URL is correct in the model settings.",
			Kind:  failureKindTransport,
		}
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"):
		return FailureInfo{
			Error: fmt.Sprintf("The request to the model server timed out: %s", bound(text, maxFailureBodyLen)),
			Hint:  "The server may be busy or still loading the model — retry, or check the server's health/logs.",
			Kind:  failureKindTimeout,
		}
	case strings.Contains(lower, "unexpected eof"), strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "broken pipe"), strings.Contains(lower, "stream error"):
		return FailureInfo{
			Error: fmt.Sprintf("The connection to the model server dropped mid-request: %s", bound(text, maxFailureBodyLen)),
			Hint:  "The server closed the connection — it may have crashed or been restarted. Check its logs and retry.",
			Kind:  failureKindTransport,
		}
	default:
		return FailureInfo{
			Error: fmt.Sprintf("The assistant run failed: %s", bound(text, maxFailureBodyLen)),
			Kind:  failureKindInternal,
		}
	}
}

// extractUpstreamMessage pulls the readable "message" out of an OpenAI-style
// error body ({... "error": {"message": "..."}}), falling back to a bounded
// raw excerpt when the body is not that shape.
func extractUpstreamMessage(body string) string {
	if body == "" {
		return "the server returned no details"
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &parsed) == nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return bound(parsed.Error.Message, maxFailureBodyLen)
	}
	return bound(body, maxFailureBodyLen)
}

// isContextOverflowBody reports whether a lowercased upstream body indicates a
// context-window overflow (400/413 from llama.cpp and most gateways).
func isContextOverflowBody(lower string) bool {
	markers := []string{
		"context length",
		"context window",
		"context is too long",
		"too large to fit in the context",
		"exceeds the context",
		"maximum context",
		"n_ctx",
		"prompt is too long",
		"reduce the length",
		"ctx_size",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// bound truncates s to at most n runes, appending an ellipsis when truncated.
func bound(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n])) + "…"
}
