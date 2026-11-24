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
	noticeWatch    = 80
)

const msgNoticeTooMany = "a notice carries too many items"
const msgNoticeEmpty = "a notice needs a watch and at least one item"

// Notice is the SDK's Notice port over the core's notify path: the stack offers structured facts and the core words the message from fixed wording,
// strips what could pass as markup or a hidden line, keeps only http and https links, and sends it as plain text to one enabled connector type
// (ADR-0036: the core words anything a stack finds). The stack's own prose never reaches the phone.
type Notice struct {
	Notify Notify
	// BriefLink is the address a phone reaching that connector can open the Briefs page at, or "" when none is known (ADR-0083); the message is then text only.
	// Nil means none.
	BriefLink func(channel string) string
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

// The bounds of a brief summary. A phone message is short, and Telegram refuses one over 4096 characters, so the summary stays well under it: the body is cut
// before the address is added, never after.
const (
	briefStories   = 5
	briefStoryLink = 300
	briefBodyBytes = 3000
)

const msgBriefEmpty = "a brief summary needs a name and at least one story or home fact"

// BriefPagePath is where the Briefs page lives in the admin UI.
const BriefPagePath = "/newsroom/briefs"

// BriefPage is the address of the Briefs page for a connector whose stored webhook address is given: that address's own origin, and only when it is https,
// since a plain address cannot be opened safely from a phone away from home. A webhook address that is empty, not https or without a host gives "" and the
// message is text only. Nothing is guessed and nothing is stored for this (ADR-0083).
func BriefPage(webhookURL string) string {
	u, err := url.Parse(strings.TrimSpace(webhookURL))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return "https://" + u.Host + BriefPagePath
}

// Brief words a written brief's summary and sends it: the brief's name, the first few top stories (a headline and a link), the home facts in the core's own
// words by id, and the address of the brief when the core knows one. Nothing is sent when it is refused.
func (n Notice) Brief(ctx context.Context, channel string, req stacksdk.BriefNoticeRequest) error {
	if len(req.Stories) > stacksdk.MaxBriefNoticeStories || len(req.Home) > stacksdk.MaxHomeLines {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNoticeTooMany)
	}
	link := ""
	if n.BriefLink != nil {
		link = webLink(n.BriefLink(channel))
	}
	text := BriefText(req, link)
	if text == "" {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefEmpty)
	}
	return n.Notify.Send(ctx, channel, text)
}

// BriefText is the message for a brief summary, empty when there is no name or nothing to say. The home facts are worded again here from the core's own
// tables by id and count: an id the core has no words for is dropped, and the sentence the stack stored is never read.
func BriefText(req stacksdk.BriefNoticeRequest, link string) string {
	name := oneLine(req.Name, noticeWatch)
	stories, shown := briefStoryLines(req.Stories)
	home := briefHomeLines(req.Home)
	if name == "" || (shown == 0 && home == "") {
		return ""
	}
	body := name
	if shown > 0 {
		body += "\n" + stories
	}
	if home != "" {
		body += "\n\nAt home" + home
	}
	body = clipBytes(strings.TrimSpace(body), briefBodyBytes)
	if link != "" {
		body += "\n\nOpen the brief: " + link
	}
	return body
}

// briefStoryLines is the first few stories as bullets with their web links, and how many there are.
func briefStoryLines(in []stacksdk.NoticeItem) (string, int) {
	var b strings.Builder
	shown := 0
	for _, s := range in {
		title := oneLine(s.Title, noticeTitle)
		if title == "" || shown == briefStories {
			continue
		}
		shown++
		b.WriteString("\n• " + title)
		if l := webLink(s.Link); l != "" && len(l) <= briefStoryLink {
			b.WriteString("\n  " + l)
		}
	}
	return b.String(), shown
}

// briefHomeLines words each home fact again from the core's own tables by id; an id the core has no words for is dropped.
func briefHomeLines(in []stacksdk.BriefNoticeHome) string {
	var b strings.Builder
	for _, h := range in {
		words, ok := HomeWords(h.ID)
		if !ok {
			continue
		}
		if c := homeCount(h.Count); c > 0 {
			fmt.Fprintf(&b, "\n• %d · %s", c, words)
		} else {
			b.WriteString("\n• " + words)
		}
	}
	return b.String()
}

// NoticeText is the message for a notice: fixed wording, then each item's title and link, then the changed facts. It is empty when there is no watch
// or no item.
func NoticeText(req stacksdk.NoticeRequest) string {
	watch := oneLine(req.Watch, noticeWatch)
	if watch == "" || len(req.Items) == 0 {
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
	fmt.Fprintf(&b, "%s: %d new", watch, len(req.Items))
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
