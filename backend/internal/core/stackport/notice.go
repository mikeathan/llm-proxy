package stackport

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"llm-proxy-stacks/sdk"
)

// The bounds of what a notice shows. The core words the message, so these are the core's, not the stack's.
const (
	noticeMaxShown = 10
	noticeTitle    = 160
	noticeFact     = 120
	noticeTopic    = 80
)

const msgNoticeTooMany = "a notice carries too many items"
const msgNoticeEmpty = "a notice needs a topic and at least one item"

// Notice is the SDK's Notice port over the core's notify path: the stack offers structured facts and the core words the message from fixed wording,
// strips what could pass as markup or a hidden line, keeps only http and https links, and sends it as plain text to one enabled connector type
// (ADR-0036: the core words anything a stack finds). The stack's own prose never reaches the phone.
type Notice struct {
	Notify Notify
}

// Send words the notice and sends it. Nothing is sent when it is refused.
func (n Notice) Send(ctx context.Context, channel string, req stacksdk.NoticeRequest) error {
	if len(req.Items) > stacksdk.MaxNoticeItems {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNoticeTooMany)
	}
	text := NoticeText(req)
	if text == "" {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNoticeEmpty)
	}
	return n.Notify.Send(ctx, channel, text)
}

// NoticeText is the message for a notice: fixed wording, then each item's title and link, then the changed facts. It is empty when there is no topic
// or no item.
func NoticeText(req stacksdk.NoticeRequest) string {
	topic := oneLine(req.Topic, noticeTopic)
	if topic == "" || len(req.Items) == 0 {
		return ""
	}
	changes := make([]stacksdk.NoticeChange, 0, len(req.Changes))
	for _, c := range req.Changes {
		// Only an item that is shown gets its changed fact, so a long list of changes cannot push the message over what a connector accepts.
		if c.Item >= 1 && c.Item <= min(len(req.Items), noticeMaxShown) {
			changes = append(changes, c)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d new", topic, len(req.Items))
	if len(changes) > 0 {
		fmt.Fprintf(&b, ", %d changed", len(changes))
	}
	for i, it := range req.Items {
		if i == noticeMaxShown {
			fmt.Fprintf(&b, "\n\n…and %d more", len(req.Items)-noticeMaxShown)
			break
		}
		b.WriteString("\n\n• " + oneLine(it.Title, noticeTitle))
		if l := webLink(it.Link); l != "" {
			b.WriteString("\n  " + l)
		}
	}
	for _, c := range changes {
		fmt.Fprintf(&b, "\n\nChanged: %s\n  was: %s\n  now: %s", oneLine(req.Items[c.Item-1].Title, noticeTitle), oneLine(c.Was, noticeFact), oneLine(c.Now, noticeFact))
	}
	return clipBytes(strings.TrimSpace(b.String()), stacksdk.MaxNotifyBytes)
}

// clipBytes cuts s to at most max bytes on a rune boundary. A notice is bounded by what it shows, so this is only the last guard: the notify port
// refuses a longer text, and a refused send must never be mistaken for a missing channel.
func clipBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// oneLine collapses all whitespace and drops control characters, so no field can add a line of its own, then clips to max runes.
func oneLine(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		s = strings.TrimSpace(string([]rune(s)[:max-1])) + "…"
	}
	return s
}

// webLink returns the link when it is an absolute http or https URL with a host and no spaces, and nothing otherwise.
func webLink(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(raw, " \t\r\n") || len(raw) > 500 {
		return ""
	}
	return raw
}
