package handlers

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
)

// replyClient answers every Chat with one fixed message; the rest of the client
// interface is never used by the completer.
type replyClient struct {
	proxy.Client
	content string
	field   string
	got     *proxy.ChatRequest
}

func (c replyClient) Chat(_ context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	if c.got != nil {
		*c.got = req
	}
	return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Content: c.content}}}}, nil
}

func (c replyClient) ReasoningField() string { return c.field }

func completerWithReply(content, _ string) chatCompleter {
	return chatCompleter{client: replyClient{content: content, field: proxy.ReasoningFieldBudget}, model: "m"}
}

// Pulling a few facts out of a chat needs no thinking, and a local thinking
// model would otherwise spend the whole allowance on it and return nothing.
func TestChatCompleter_LocalModelDoesNotThink(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		wantOff     bool
	}{
		{"local llama.cpp client", proxy.ReasoningFieldThinkTokens, true},
		{"cloud client", proxy.ReasoningFieldBudget, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got proxy.ChatRequest
			c := chatCompleter{client: replyClient{content: "[]", field: tc.field, got: &got}, model: "m"}
			if _, err := c.Complete(context.Background(), "sys", "user"); err != nil {
				t.Fatalf("Complete: %v", err)
			}
			off := got.ChatTemplateKwargs != nil && !got.ChatTemplateKwargs.EnableThinking
			if off != tc.wantOff {
				t.Errorf("thinking disabled = %v, want %v (request %+v)", off, tc.wantOff, got)
			}
		})
	}
}

// A thinking model can spend the whole token allowance reasoning and return no
// answer; that is a failed review, not "no suggestions".
func TestChatCompleter_EmptyAnswerIsAnError(t *testing.T) {
	_, err := completerWithReply("", "length").Complete(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "no answer") {
		t.Fatalf("err = %v, want a no-answer error", err)
	}
	if out, err := completerWithReply(`[]`, "stop").Complete(context.Background(), "sys", "user"); err != nil || out != "[]" {
		t.Errorf("a real answer must pass through, got %q, %v", out, err)
	}
}
