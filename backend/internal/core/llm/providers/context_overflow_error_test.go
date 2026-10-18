package providers

import (
	"errors"
	"testing"
)

func TestParseContextOverflowError(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		overflow     bool
		prompt, size int
	}{
		{"llama.cpp reports both counts as fields",
			`{"error":{"code":400,"message":"request (16401 tokens) exceeds the available context size (16384 tokens), try increasing it","type":"exceed_context_size_error","n_prompt_tokens":16401,"n_ctx":16384}}`,
			true, 16401, 16384},
		{"OpenAI says window then prompt",
			`{"error":{"message":"This model's maximum context length is 128000 tokens. However, your messages resulted in 130500 tokens. Please reduce the length of the messages.","code":"context_length_exceeded"}}`,
			true, 130500, 128000},
		{"Anthropic says prompt then window",
			`{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000 maximum"}}`,
			true, 210000, 200000},
		{"Gemini says prompt then window",
			`{"error":{"message":"The input token count (1200000) exceeds the maximum number of tokens allowed (1048576)."}}`,
			true, 1200000, 1048576},
		{"recognised without counts",
			`{"error":{"message":"Context length exceeded, reduce the length of your input"}}`,
			true, 0, 0},
		{"an unrelated 400", `{"error":{"message":"temperature must be between 0 and 2"}}`, false, 0, 0},
		{"an output-cap 400 is not an overflow", `{"error":{"message":"max_tokens is greater than the maximum allowed 8192"}}`, false, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContextOverflowError(tc.body)
			if (got != nil) != tc.overflow {
				t.Fatalf("overflow = %v, want %v", got != nil, tc.overflow)
			}
			if got == nil {
				return
			}
			if got.PromptTokens != tc.prompt || got.WindowTokens != tc.size {
				t.Errorf("counts = (%d, %d), want (%d, %d)", got.PromptTokens, got.WindowTokens, tc.prompt, tc.size)
			}
			if !errors.Is(got, ErrContextOverflow) {
				t.Error("must wrap ErrContextOverflow")
			}
		})
	}
}
