// errors.go — LLM-error recovery classification for the agent loop.
//
// The agent loop inspects upstream/provider errors to decide HOW to recover
// (retry in XML mode, drop a rejected parameter, nag after a context
// overflow, …). These pure string/type classifiers live beside
// ClassifyRunFailure because they share the same domain — turning raw LLM
// failures into decisions — even though they drive recovery routing rather
// than the UI-facing summaries in this package's failure.go.
package failures

import (
	"strings"

	"llm-proxy/internal/core/proxy"
)

// ParseErrorKind classifies a ParseError into a stable category so the
// escalation logic can detect when the model keeps making the same mistake.
func ParseErrorKind(e *proxy.ParseError) string {
	if e == nil {
		return ""
	}
	if !e.XMLFound {
		return "no_xml"
	}
	if e.JSONError != "" {
		return "json"
	}
	if e.ToolName != "" {
		return "tool_name"
	}
	return ""
}

// IsTruncationError reports whether an error string indicates the model's
// output was cut off mid-JSON (stream truncation), which the loop treats as a
// truncated tool-call attempt rather than a complete answer.
func IsTruncationError(errStr string) bool {
	low := strings.ToLower(errStr)
	return strings.Contains(low, "unexpected end") || strings.Contains(low, "missing closing")
}

// IsToolCallParseError reports whether the upstream failed to parse a tool
// call's arguments as JSON (llama.cpp surfaces this as a 500 with
// "Failed to parse tool call arguments as JSON").
func IsToolCallParseError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "failed to parse tool call arguments")
}

// IsJSONSyntaxError reports whether the error is a server-side JSON syntax
// failure that a retry with corrected args can clear.
func IsJSONSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "missing closing") ||
		strings.Contains(low, "invalid string") ||
		strings.Contains(low, "unexpected end") ||
		strings.Contains(low, "parse error")
}

// IsContextSizeError reports whether the request exceeded the model's context
// window (drives the physical-sieve recovery, not a UI message).
func IsContextSizeError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "context size") ||
		strings.Contains(low, "context_length_exceeded") ||
		strings.Contains(low, "maximum context length") ||
		strings.Contains(low, "reduce the length") ||
		strings.Contains(low, "too many tokens") ||
		strings.Contains(low, "reasoning stuck")
}

// IsToolSupportError reports whether the endpoint rejected the tools
// parameter outright (model without tool-calling support).
func IsToolSupportError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "tools is not currently supported") ||
		strings.Contains(lowErr, "tool_choice is not supported") ||
		strings.Contains(lowErr, "auto tool choice requires") ||
		strings.Contains(lowErr, "parameter `tools`")
}

// IsUnsupportedParameterError reports whether the endpoint rejected a request
// parameter it does not know (e.g. thinking_budget_tokens on a non-thinking
// gateway) — the parameter is dropped and the request retried.
func IsUnsupportedParameterError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "unsupported parameter")
}

// IsPrefillThinkingError reports whether the server rejected the assistant
// prefill because thinking mode is active (prefill + thinking are
// incompatible on llama.cpp) — the retry drops the prefill.
func IsPrefillThinkingError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "prefill") &&
		strings.Contains(lowErr, "thinking")
}
