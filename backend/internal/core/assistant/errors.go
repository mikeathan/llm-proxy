// errors.go — Error classification helpers used by the agent loop to decide
// how to recover from LLM failures (context size, parse errors, tool support,
// prefill thinking rejection).
package assistant

import (
	"strings"

	"llm-proxy/internal/core/proxy"
)

// parseErrorKind classifies a ParseError into a stable category so the
// escalation logic can detect when the model keeps making the same mistake.
func parseErrorKind(e *proxy.ParseError) string {
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

func isTruncationError(errStr string) bool {
	low := strings.ToLower(errStr)
	return strings.Contains(low, "unexpected end") || strings.Contains(low, "missing closing")
}

func isToolCallParseError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "failed to parse tool call arguments")
}

func isContextSizeError(err error) bool {
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

func isToolSupportError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "tools is not currently supported") ||
		strings.Contains(lowErr, "tool_choice is not supported") ||
		strings.Contains(lowErr, "auto tool choice requires") ||
		strings.Contains(lowErr, "parameter `tools`")
}

func isPrefillThinkingError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "prefill") &&
		strings.Contains(lowErr, "thinking")
}
