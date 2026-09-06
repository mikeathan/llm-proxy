package failures

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
)

func TestClassifyRunFailure(t *testing.T) {
	httpErr := func(kind string, status int, body string) *proxy.LLMHTTPError {
		return &proxy.LLMHTTPError{Kind: kind, StatusCode: status, Body: body}
	}

	tests := []struct {
		name      string
		err       error
		wantKind  string
		wantSub   []string // substrings expected in Error
		wantHint  []string // substrings expected in Hint ("" case = no hint)
		wantNoSub []string // substrings that must NOT appear (raw JSON dumps)
		wantEmpty bool     // Error must be empty (nil err only)
	}{
		{
			name:      "nil error",
			err:       nil,
			wantKind:  failureKindInternal,
			wantEmpty: true,
		},
		{
			name:     "user cancel",
			err:      context.Canceled,
			wantKind: failureKindCancel,
			wantSub:  []string{"cancelled"},
		},
		{
			name:     "deadline exceeded",
			err:      context.DeadlineExceeded,
			wantKind: failureKindTimeout,
			wantSub:  []string{"timed out"},
		},
		{
			name:      "grammar+tools 400 (chat)",
			err:       httpErr("chat", 400, `{"error":{"code":400,"message":"Cannot use custom grammar constraints with tools.","type":"invalid_request_error"}}`),
			wantKind:  failureKindChat,
			wantSub:   []string{"HTTP 400", "Cannot use custom grammar constraints with tools."},
			wantHint:  []string{"grammar", "outdated"},
			wantNoSub: []string{`{"error"`},
		},
		{
			name:     "grammar+tools 400 (stream)",
			err:      httpErr("stream", 400, `{"error":{"message":"Cannot use custom grammar constraints with tools."}}`),
			wantKind: failureKindStream,
			wantSub:  []string{"Cannot use custom grammar constraints with tools."},
			wantHint: []string{"llama.cpp"},
		},
		{
			name:     "context overflow",
			err:      httpErr("chat", 400, `{"error":{"message":"Prompt is too long: 20000 tokens exceeds the context window of 16384"}}`),
			wantKind: failureKindChat,
			wantSub:  []string{"HTTP 400"},
			wantHint: []string{"context"},
		},
		{
			name:     "unauthorized",
			err:      httpErr("chat", 401, `{"error":{"message":"invalid api key"}}`),
			wantKind: failureKindChat,
			wantHint: []string{"API key"},
		},
		{
			name:     "model not found",
			err:      httpErr("chat", 404, `{"error":{"message":"model not found"}}`),
			wantKind: failureKindChat,
			wantHint: []string{"model name"},
		},
		{
			name:     "rate limited",
			err:      httpErr("chat", 429, `{"error":{"message":"rate limit exceeded"}}`),
			wantKind: failureKindChat,
			wantHint: []string{"rate-limiting"},
		},
		{
			name:     "service unavailable",
			err:      httpErr("chat", 503, `{"error":{"message":"overloaded"}}`),
			wantKind: failureKindChat,
			wantHint: []string{"temporarily unavailable"},
		},
		{
			name:     "generic http error extracts message",
			err:      httpErr("chat", 500, `{"error":{"message":"internal engine error"}}`),
			wantKind: failureKindChat,
			wantSub:  []string{"HTTP 500", "internal engine error"},
		},
		{
			name:     "wrapped http error is found via errors.As",
			err:      fmt.Errorf("llm completion failed: %w", httpErr("chat", 400, `{"error":{"message":"Cannot use custom grammar constraints with tools."}}`)),
			wantKind: failureKindChat,
			wantHint: []string{"grammar"},
		},
		{
			name:     "connection refused",
			err:      errors.New(`Post "http://127.0.0.1:8081/v1/chat/completions": dial tcp 127.0.0.1:8081: connect: connection refused`),
			wantKind: failureKindTransport,
			wantSub:  []string{"Could not reach the model server"},
			wantHint: []string{"running"},
		},
		{
			name:     "header timeout",
			err:      errors.New(`Post "https://integrate.api.nvidia.com/v1/chat/completions": net/http: timeout awaiting response headers`),
			wantKind: failureKindTimeout,
			wantSub:  []string{"timed out"},
			wantHint: []string{"busy or still loading"},
		},
		{
			name:     "connection reset",
			err:      errors.New("LLM stream error: unexpected EOF"),
			wantKind: failureKindTransport,
			wantSub:  []string{"connection to the model server dropped"},
			wantHint: []string{"logs"},
		},
		{
			name:     "unknown error stays bounded",
			err:      errors.New(strings.Repeat("y", 5000)),
			wantKind: failureKindInternal,
			wantSub:  []string{"assistant run failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := ClassifyRunFailure(tt.err)
			if fi.Kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", fi.Kind, tt.wantKind)
			}
			if tt.wantEmpty {
				if fi.Error != "" {
					t.Errorf("expected empty Error, got %q", fi.Error)
				}
				return
			}
			if fi.Error == "" {
				t.Fatal("expected non-empty Error")
			}
			for _, sub := range tt.wantSub {
				if !strings.Contains(fi.Error, sub) {
					t.Errorf("Error %q missing %q", fi.Error, sub)
				}
			}
			for _, sub := range tt.wantNoSub {
				if strings.Contains(fi.Error, sub) {
					t.Errorf("Error %q must not contain %q", fi.Error, sub)
				}
			}
			for _, sub := range tt.wantHint {
				if !strings.Contains(fi.Hint, sub) {
					t.Errorf("Hint %q missing %q", fi.Hint, sub)
				}
			}
			// The UI-facing Error must never carry an unbounded raw body.
			if n := len([]rune(fi.Error)); n > 400 {
				t.Errorf("Error too long for UI: %d runes", n)
			}
		})
	}
}
