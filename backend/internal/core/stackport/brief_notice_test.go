package stackport

import (
	"context"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

type sink struct{ sent []string }

func (s *sink) send(_ context.Context, _, text string) error {
	s.sent = append(s.sent, text)
	return nil
}

func briefNotice(link string) (Notice, *sink) {
	s := &sink{}
	return Notice{Notify: Notify{Types: func() []string { return []string{"telegram"} }, Deliver: s.send}, BriefLink: func(string) string { return link }}, s
}

func TestBriefText_IsShortPlainAndWordedByTheCore(t *testing.T) {
	req := stacksdk.BriefNoticeRequest{Name: "Morning",
		Stories: []stacksdk.NoticeItem{{Title: "Qwen4 out", Link: "https://q.example/4"}, {Title: "Headline only"}},
		Home:    []stacksdk.BriefNoticeHome{{ID: HomeNetworkOnline, Count: 16}, {ID: "radio_pmf_off", Count: 1}, {ID: "made_up", Count: 3}}}
	got := BriefText(req, "https://home.example/newsroom/briefs")
	for _, want := range []string{"Morning\n\n• Qwen4 out\n  https://q.example/4\n• Headline only", "At home\n• 16 · Devices the network sees right now.", "Open the brief: https://home.example/newsroom/briefs"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "made_up") || strings.Contains(got, "3 ·") {
		t.Errorf("an id the core has no words for is dropped:\n%s", got)
	}
}

// What the stack stored as the sentence of a home line is never read: only the id and the count cross, and the words are the core's.
func TestBriefText_TheCoreRewordsHomeLinesFromTheirIDs(t *testing.T) {
	got := BriefText(stacksdk.BriefNoticeRequest{Name: "M", Home: []stacksdk.BriefNoticeHome{{ID: HomeNetworkOnline, Count: 2}}}, "")
	if !strings.Contains(got, homeWords[HomeNetworkOnline]) || strings.Contains(got, "Open the brief") {
		t.Errorf("%s", got)
	}
}

// A critical fact is the first thing on the phone, above the headlines, and is not said twice under "At home".
func TestBriefText_ACriticalFactComesFirstAndOnlyOnce(t *testing.T) {
	req := stacksdk.BriefNoticeRequest{Name: "Morning",
		Stories: []stacksdk.NoticeItem{{Title: "Qwen4 out"}},
		Home:    []stacksdk.BriefNoticeHome{{ID: HomeNetworkOnline, Count: 16}, {ID: "radio_open_or_weak_ap", Count: 1}}}
	got := BriefText(req, "")
	wantTop := "Morning\n⚠ 1 · " + radiodigestWords(t, "radio_open_or_weak_ap") + "\n\n• Qwen4 out"
	if !strings.HasPrefix(got, wantTop) {
		t.Errorf("want the warning right under the name:\n%s", got)
	}
	if strings.Count(got, radiodigestWords(t, "radio_open_or_weak_ap")) != 1 {
		t.Errorf("said twice:\n%s", got)
	}
	if !strings.Contains(got, "At home\n• 16 · Devices the network sees right now.") {
		t.Errorf("the routine facts stay under At home:\n%s", got)
	}
	if quiet := BriefText(stacksdk.BriefNoticeRequest{Name: "M", Stories: req.Stories, Home: req.Home[:1]}, ""); strings.Contains(quiet, "⚠") {
		t.Errorf("a quiet day has no warning:\n%s", quiet)
	}
}

// The phone message says a radio finding in its short sentence, never the full one the card and the Brief page show.
func TestBriefText_ARadioFactIsSaidInItsShortSentence(t *testing.T) {
	got := BriefText(stacksdk.BriefNoticeRequest{Name: "M", Home: []stacksdk.BriefNoticeHome{{ID: "radio_pmf_off", Count: 1}}}, "")
	full, _ := HomeWords("radio_pmf_off")
	short, _ := HomeShortWords("radio_pmf_off")
	if !strings.Contains(got, short) || strings.Contains(got, full) || len(short) >= len(full) {
		t.Errorf("short %q, full %q in:\n%s", short, full, got)
	}
}

func radiodigestWords(t *testing.T, id string) string {
	t.Helper()
	w, ok := HomeShortWords(id)
	if !ok {
		t.Fatalf("no words for %s", id)
	}
	return w
}

func TestBriefText_NeedsANameAndSomethingToSay(t *testing.T) {
	for name, req := range map[string]stacksdk.BriefNoticeRequest{
		"no name":    {Stories: []stacksdk.NoticeItem{{Title: "x"}}},
		"no content": {Name: "Morning"},
		"only junk":  {Name: "Morning", Stories: []stacksdk.NoticeItem{{Title: " \n "}}, Home: []stacksdk.BriefNoticeHome{{ID: "made_up"}}},
	} {
		if got := BriefText(req, ""); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestBriefText_StaysUnderWhatAPhoneMessageAllowsAndKeepsTheAddress(t *testing.T) {
	req := stacksdk.BriefNoticeRequest{Name: strings.Repeat("N", 500)}
	for range stacksdk.MaxBriefNoticeStories {
		req.Stories = append(req.Stories, stacksdk.NoticeItem{Title: strings.Repeat("é", 400), Link: "https://x.example/" + strings.Repeat("a", 250)})
	}
	got := BriefText(req, "https://home.example/newsroom/briefs")
	if len(got) > 4000 || !strings.HasSuffix(got, "Open the brief: https://home.example/newsroom/briefs") {
		t.Errorf("%d bytes, ends %q", len(got), got[max(0, len(got)-60):])
	}
	if strings.Count(got, "•") > briefStories {
		t.Errorf("more than %d stories:\n%s", briefStories, got)
	}
}

func TestBriefPage_IsTheHTTPSOriginOfTheConnectorsWebhookOrNothing(t *testing.T) {
	for in, want := range map[string]string{
		"https://home.example.com/api/v1/webhooks/telegram":      "https://home.example.com/newsroom/briefs",
		"https://home.example.com:8443/api/v1/webhooks/telegram": "https://home.example.com:8443/newsroom/briefs",
		"http://192.168.1.5:4001/api/v1/webhooks/telegram":       "",
		"":                                   "",
		"not a url":                          "",
		"https://user:pw@home.example.com/x": "",
		"javascript:alert(1)":                "",
	} {
		if got := BriefPage(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestNoticeBrief_SendsOnlyToAnEnabledChannelAndOnlyAWebAddress(t *testing.T) {
	n, s := briefNotice("javascript:alert(1)")
	req := stacksdk.BriefNoticeRequest{Name: "Morning", Stories: []stacksdk.NoticeItem{{Title: "x"}}}
	if err := n.Brief(context.Background(), "telegram", req); err != nil || len(s.sent) != 1 || strings.Contains(s.sent[0], "Open the brief") {
		t.Fatalf("%v %q", err, s.sent)
	}
	if err := n.Brief(context.Background(), "slack", req); stacksdk.CodeOf(err) != stacksdk.CodeInvalid || len(s.sent) != 1 {
		t.Errorf("a channel that is not enabled is refused: %v", err)
	}
	if err := n.Brief(context.Background(), "telegram", stacksdk.BriefNoticeRequest{Name: "Morning"}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("an empty summary is refused: %v", err)
	}
	over := stacksdk.BriefNoticeRequest{Name: "M", Stories: make([]stacksdk.NoticeItem, stacksdk.MaxBriefNoticeStories+1)}
	if err := n.Brief(context.Background(), "telegram", over); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("too many stories are refused, not cut: %v", err)
	}
}
