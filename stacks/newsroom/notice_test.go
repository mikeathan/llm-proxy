package newsroom

import (
	"context"
	"errors"
	"testing"

	stacksdk "llm-proxy-stacks/sdk"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/newsroom/watches"
)

type fakeNotice struct {
	got     stacksdk.NoticeRequest
	channel string
	err     error
}

func (f *fakeNotice) Send(_ context.Context, channel string, n stacksdk.NoticeRequest) error {
	f.channel, f.got = channel, n
	return f.err
}

// Brief is the summary port: the tests of the Newsroom stack's own notices do not use it.
func (f *fakeNotice) Brief(context.Context, string, stacksdk.BriefNoticeRequest) error { return f.err }

func TestNoticeSender_HandsTheCoreFactsNotWords(t *testing.T) {
	f := &fakeNotice{}
	send := noticeSender(f)
	n := watches.Notice{Watch: "Go", Items: []sources.Item{{Title: "1.26", Link: "https://go.dev/1.26", Text: "body the core never sees"}}, Changes: []watches.Change{{Item: 1, Was: "rc", Now: "final"}}}
	if err := send(context.Background(), watches.Watch{Channel: "telegram"}, n); err != nil {
		t.Fatal(err)
	}
	if f.channel != "telegram" || f.got.Watch != "Go" || len(f.got.Items) != 1 || f.got.Items[0].Title != "1.26" || f.got.Items[0].Link != "https://go.dev/1.26" ||
		len(f.got.Changes) != 1 || f.got.Changes[0].Now != "final" {
		t.Errorf("channel %q, request %+v", f.channel, f.got)
	}
}

func TestNoticeSender_ARefusedChannelIsNoChannelAndAFailureStaysAFailure(t *testing.T) {
	n := watches.Notice{Watch: "Go", Items: []sources.Item{{Title: "x"}}}
	refused := noticeSender(&fakeNotice{err: stacksdk.Errorf(stacksdk.CodeInvalid, "that channel is not an enabled connector")})
	if err := refused(context.Background(), watches.Watch{Channel: "email"}, n); !errors.Is(err, watches.ErrNoChannel) {
		t.Errorf("want no channel, got %v", err)
	}
	down := noticeSender(&fakeNotice{err: stacksdk.Errorf(stacksdk.CodeUnavailable, "down")})
	if err := down(context.Background(), watches.Watch{Channel: "telegram"}, n); err == nil || errors.Is(err, watches.ErrNoChannel) {
		t.Errorf("a connector failure must stay a failure so the next run retries, got %v", err)
	}
	if noticeSender(nil) != nil {
		t.Error("with no port there is no sender")
	}
}
