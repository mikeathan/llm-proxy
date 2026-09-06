package failures

import (
	"fmt"
	"testing"

	"llm-proxy/internal/core/proxy"
)

// These unit tests moved verbatim from the assistant package (agent_test.go)
// together with the recovery-classification helpers they cover.

func TestIsToolCallParseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "empty error",
			err:  fmt.Errorf(""),
			want: false,
		},
		{
			name: "arbitrary error",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "llama.cpp tool call parse error (stream)",
			err:  fmt.Errorf(`llm completion failed: LLM stream error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error at line 1, column 730: syntax error while parsing value - invalid string: missing closing quote"}}`),
			want: true,
		},
		{
			name: "llama.cpp tool call parse error (non-stream)",
			err:  fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error"}}`),
			want: true,
		},
		{
			name: "wrapped error with tool call parse",
			err:  fmt.Errorf("something went wrong: LLM chat error 500: {\"error\":{\"message\":\"Failed to parse tool call arguments as JSON: unexpected end of input\"}}"),
			want: true,
		},
		{
			name: "context size error should not match",
			err:  fmt.Errorf("llm completion failed: context size exceeded"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsToolCallParseError(tt.err)
			if got != tt.want {
				t.Errorf("IsToolCallParseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPrefillThinkingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no prefill", fmt.Errorf("connection reset"), false},
		{"prefill but no thinking", fmt.Errorf("prefill failed: token limit"), false},
		{"prefill + thinking (lowercase)", fmt.Errorf("prefill with thinking mode not supported"), true},
		{"prefill + thinking (mixed)", fmt.Errorf("prefill rejected: Thinking mode active"), true},
		{"prefill + thinking (uppercase)", fmt.Errorf("PREFILL THINKING ERROR"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPrefillThinkingError(tt.err)
			if got != tt.want {
				t.Errorf("IsPrefillThinkingError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsToolSupportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"tools not supported", fmt.Errorf("parameter `tools` is not currently supported"), true},
		{"tool choice not supported", fmt.Errorf("tool_choice is not supported with this model"), true},
		{"auto tool choice", fmt.Errorf("auto tool choice requires function calling"), true},
		{"tools in params", fmt.Errorf("parameter `tools` is not allowed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsToolSupportError(tt.err)
			if got != tt.want {
				t.Errorf("IsToolSupportError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsContextSizeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"context size", fmt.Errorf("request exceeds the available context size"), true},
		{"context length exceeded", fmt.Errorf("context_length_exceeded"), true},
		{"max context length", fmt.Errorf("maximum context length exceeded"), true},
		{"reduce the length", fmt.Errorf("please reduce the length of the messages"), true},
		{"too many tokens", fmt.Errorf("too many tokens for this model"), true},
		{"token limit", fmt.Errorf("token limit reached"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsContextSizeError(tt.err)
			if got != tt.want {
				t.Errorf("IsContextSizeError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnsupportedParameterError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"nvidia real error", fmt.Errorf(`LLM chat error 400: {"error":{"message":"Validation: Unsupported parameter(s): ` + "`thinking_budget_tokens`" + `","type":"Bad Request","code":400}}`), true},
		{"simple unsupported", fmt.Errorf("Unsupported parameter: thinking_budget_tokens"), true},
		{"tool support error not matched", fmt.Errorf("tools is not currently supported"), false},
		{"context size error not matched", fmt.Errorf("context size exceeded"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUnsupportedParameterError(tt.err)
			if got != tt.want {
				t.Errorf("IsUnsupportedParameterError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTruncationError(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{"empty", "", false},
		{"arbitrary", "something went wrong", false},
		{"unexpected end", "unexpected end of JSON input", true},
		{"missing closing", "missing closing quote", true},
		{"unexpected end in long message", "parse error: unexpected end of input while parsing", true},
		{"missing closing in wrapped", "wrapper: missing closing delimiter", true},
		{"no match", "invalid character", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTruncationError(tt.err)
			if got != tt.want {
				t.Errorf("IsTruncationError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParseErrorKind(t *testing.T) {
	tests := []struct {
		name string
		pe   *proxy.ParseError
		want string
	}{
		{"nil", nil, ""},
		{"no xml", &proxy.ParseError{XMLFound: false, JSONError: "", ToolName: ""}, "no_xml"},
		{"json error", &proxy.ParseError{XMLFound: true, JSONError: "invalid json", ToolName: ""}, "json"},
		{"tool name", &proxy.ParseError{XMLFound: true, JSONError: "", ToolName: "bad_tool"}, "tool_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseErrorKind(tt.pe)
			if got != tt.want {
				t.Errorf("ParseErrorKind() = %v, want %v", got, tt.want)
			}
		})
	}
}
