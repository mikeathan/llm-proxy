package assistant

import (
	"testing"
)

func TestFilterStreamingMarkup(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContent  string
		wantToolCall bool
	}{
		{
			name:         "no markup",
			input:        "Hello, I am thinking about this.",
			wantContent:  "Hello, I am thinking about this.",
			wantToolCall: false,
		},
		{
			name:         "starts with tools",
			input:        "Hello <tools>call",
			wantContent:  "Hello ",
			wantToolCall: true,
		},
		{
			name:         "json block",
			input:        "Let me check. ```json\n{",
			wantContent:  "Let me check. ",
			wantToolCall: true,
		},
		{
			name:         "xml tags",
			input:        "Here we go <function-name>scan",
			wantContent:  "Here we go ",
			wantToolCall: true,
		},
		{
			name:         "xml args tag",
			input:        "Args: <args-json-object>{",
			wantContent:  "Args: ",
			wantToolCall: true,
		},
		{
			name:         "python dict",
			input:        "Okay [{'type': 'function'",
			wantContent:  "Okay ",
			wantToolCall: true,
		},
		// Edge cases — normal text that should NOT be truncated
		{
			name:         "math less-than symbol",
			input:        "The value 5 < 10 is correct.",
			wantContent:  "The value 5 < 10 is correct.",
			wantToolCall: false,
		},
		{
			name:         "set notation with braces",
			input:        "Consider the set {1, 2, 3} for this.",
			wantContent:  "Consider the set {1, 2, 3} for this.",
			wantToolCall: false,
		},
		{
			name:         "JSON in normal sentence",
			input:        `The format is {"key": "value"} for the config.`,
			wantContent:  `The format is {"key": "value"} for the config.`,
			wantToolCall: false,
		},
		{
			name:         "markdown code fence with content after",
			input:        "Here is code:\n```\nprint('hello')\n```\nNow continuing...",
			wantContent:  "Here is code:\n",
			wantToolCall: true,
		},
		{
			name:         "normal text with functions word",
			input:        "The system uses several functions.",
			wantContent:  "The system uses several ",
			wantToolCall: true,
		},
		{
			name:         "tool_call tag mid-content",
			input:        "Processing... <tool_call>data</tool_call> done.",
			wantContent:  "Processing... ",
			wantToolCall: true,
		},
		{
			name:         "pipe-style tool call",
			input:        "Starting <|tool_call>query</|tool_call> now.",
			wantContent:  "Starting ",
			wantToolCall: true,
		},
		{
			name:         "JSON name field mid-content",
			input:        `Response: {"name": "example"}`,
			wantContent:  "Response: ",
			wantToolCall: true,
		},
		{
			name:         "normal text with quoted type",
			input:        `The "type" of object matters.`,
			wantContent:  `The "type" of object matters.`,
			wantToolCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotToolCall := FilterStreamingMarkup(tt.input)
			if gotContent != tt.wantContent {
				t.Errorf("FilterStreamingMarkup() gotContent = %v, want %v", gotContent, tt.wantContent)
			}
			if gotToolCall != tt.wantToolCall {
				t.Errorf("FilterStreamingMarkup() gotToolCall = %v, want %v", gotToolCall, tt.wantToolCall)
			}
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "Hello",
			expected: "Hello",
		},
		{
			name:     "python empty block glitch",
			input:    "[{'type': 'text', 'text': ''}] Hello",
			expected: "Hello",
		},
		{
			name:     "json empty block glitch",
			input:    "[{\"type\": \"text\", \"text\": \"\"}] Hello",
			expected: "Hello",
		},
		{
			name:     "single object glitch",
			input:    "{'type': 'text', 'text': ''} Hello",
			expected: "Hello",
		},
		{
			name:     "no space",
			input:    "[{'type': 'text', 'text': ''}]<tools>call</tools>",
			expected: "<tools>call</tools>",
		},
		{
			name:     "embedded",
			input:    "Thinking... [{'type': 'text', 'text': ''}]",
			expected: "Thinking...", // Should trim the end too
		},
		{
			name:     "space inside empty string",
			input:    "[{'type': 'text', 'text': ' '}] Hello",
			expected: "Hello",
		},
		{
			name:     "incomplete block",
			input:    "[{'type': 'text', 'text': '' Hello",
			expected: "Hello",
		},
		{
			name:     "incomplete block with space",
			input:    "[{'type': 'text', 'text': ' ' Hello",
			expected: "Hello",
		},
		{
			name:     "extract text from single quote python block",
			input:    "[{'type': 'text', 'text': 'Perfect. Now let me begin Phase 1: Rapid Host Discovery.'}]",
			expected: "Perfect. Now let me begin Phase 1: Rapid Host Discovery.",
		},
		{
			name:     "extract text from double quote json block",
			input:    "[{\"type\": \"text\", \"text\": \"Valid JSON format\"}]",
			expected: "Valid JSON format",
		},
		{
			name:     "extract text from truncated block",
			input:    "[{'type': 'text', 'text': 'Some text'",
			expected: "Some text",
		},
		{
			name:     "mixed block and normal text",
			input:    "[{'type': 'text', 'text': 'Hello'}] World",
			expected: "Hello World",
		},
		{
			name:     "extract text with embedded newlines",
			input:    "[{'type': 'text', 'text': 'Line 1\nLine 2'}]",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "extract text missing brackets",
			input:    "{'type': 'text', 'text': 'No brackets'}",
			expected: "No brackets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContent(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeContent(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
