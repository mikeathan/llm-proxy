package stackport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"llm-proxy-stacks/sdk"
)

func TestNoticeText_IsFixedWording(t *testing.T) {
	req := stacksdk.NoticeRequest{Watch: "LLMs", Items: []stacksdk.NoticeItem{{Title: "Qwen4 out", Link: "https://q.example/4"}, {Title: "Llama 5", Link: "https://m.example/5"}},
		Changes: []stacksdk.NoticeChange{{Item: 2, Was: "beta", Now: "stable"}}}
	want := "LLMs: 2 new, 1 changed\n\n• Qwen4 out\n  https://q.example/4\n\n• Llama 5\n  https://m.example/5\n\nChanged: Llama 5\n  was: beta\n  now: stable"
	if got := NoticeText(req); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	many := stacksdk.NoticeRequest{Watch: "T"}
	for range noticeMaxShown + 4 {
		many.Items = append(many.Items, stacksdk.NoticeItem{Title: "i"})
	}
	if !strings.Contains(NoticeText(many), "…and 4 more") {
		t.Errorf("a long notice is cut with a count:\n%s", NoticeText(many))
	}
}

// What a source or a model wrote may not add a line, a control character or a link that is not a web link to the owner's phone.
func TestNoticeText_NoFieldAddsALineOrAScheme(t *testing.T) {
	req := stacksdk.NoticeRequest{Watch: "T\nFAKE: pay now", Items: []stacksdk.NoticeItem{{Title: "a\n\nb\x07c", Link: "javascript:alert(1)"}, {Title: "ok", Link: "https://x.example/a b"}},
		Changes: []stacksdk.NoticeChange{{Item: 1, Was: "x\ny", Now: "z"}, {Item: 9, Was: "out", Now: "of range"}}}
	got := NoticeText(req)
	for _, bad := range []string{"javascript:", "https://x.example/a b", "out", "\x07", "T\nFAKE"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q must not appear in:\n%s", bad, got)
		}
	}
	if !strings.Contains(got, "T FAKE: pay now: 2 new, 1 changed") || !strings.Contains(got, "• a bc") {
		t.Errorf("fields are one line each:\n%s", got)
	}
}

// The worst notice a source can make must still fit what a connector accepts, or the send would be refused and retried for ever.
func TestNoticeText_TheWorstNoticeFitsTheLimitOfAMessage(t *testing.T) {
	req := stacksdk.NoticeRequest{Watch: strings.Repeat("題", 200)}
	link := "https://x.example/" + strings.Repeat("a", 480)
	for i := range stacksdk.MaxNoticeItems {
		req.Items = append(req.Items, stacksdk.NoticeItem{Title: strings.Repeat("題", 400), Link: link})
		req.Changes = append(req.Changes, stacksdk.NoticeChange{Item: i + 1, Was: strings.Repeat("變", 300), Now: strings.Repeat("變", 300)})
	}
	got := NoticeText(req)
	if len(got) == 0 || len(got) > stacksdk.MaxNotifyBytes || !utf8.ValidString(got) {
		t.Errorf("len %d, valid utf-8 %v, limit %d", len(got), utf8.ValidString(got), stacksdk.MaxNotifyBytes)
	}
	var sent int
	n := Notice{Notify: Notify{Types: func() []string { return []string{"telegram"} }, Deliver: func(context.Context, string, string) error { sent++; return nil }}}
	if err := n.Send(context.Background(), "telegram", req); err != nil || sent != 1 {
		t.Errorf("the worst notice must be sent, got %v, sent %d", err, sent)
	}
}

func TestNotice_SendsOnlyToAnEnabledChannelAndRefusesAnEmptyOne(t *testing.T) {
	var sent []string
	n := Notice{Notify: Notify{Types: func() []string { return []string{"telegram"} },
		Deliver: func(_ context.Context, ch, text string) error { sent = append(sent, ch+"|"+text); return nil }}}
	req := stacksdk.NoticeRequest{Watch: "T", Items: []stacksdk.NoticeItem{{Title: "one"}}}
	if err := n.Send(context.Background(), "email", req); stacksdk.CodeOf(err) != stacksdk.CodeInvalid || len(sent) != 0 {
		t.Errorf("a channel that is not enabled is refused, got %v, sent %v", err, sent)
	}
	if err := n.Send(context.Background(), "telegram", stacksdk.NoticeRequest{Watch: "T"}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("a notice with no item is refused, got %v", err)
	}
	if err := n.Send(context.Background(), "telegram", stacksdk.NoticeRequest{Watch: "T", Items: make([]stacksdk.NoticeItem, stacksdk.MaxNoticeItems+1)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("too many items are refused, got %v", err)
	}
	if err := n.Send(context.Background(), "telegram", req); err != nil || len(sent) != 1 || sent[0] != "telegram|T: 1 new\n\n• one" {
		t.Errorf("send = %v, %v", err, sent)
	}
	bad := Notice{Notify: Notify{Types: func() []string { return []string{"telegram"} }, Deliver: func(context.Context, string, string) error { return errors.New("down") }}}
	if err := bad.Send(context.Background(), "telegram", req); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a connector failure is unavailable, got %v", err)
	}
}
