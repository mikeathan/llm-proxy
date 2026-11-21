package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// The look-up behind the watch form: before a source is saved the owner can see what it would read. It reuses the adapters' own address checks, URLs and
// parsers, so a preview shows exactly what a run would. It is a hint, never a requirement: saving still validates on its own.

// SampleSize is how many items a preview shows.
const SampleSize = 3

// suggestLimit is how many choices a suggestion lists.
const suggestLimit = 8

// Sample is one item of a preview.
type Sample struct {
	Title string    `json:"title"`
	Link  string    `json:"link,omitempty"`
	Text  string    `json:"text,omitempty"`
	At    time.Time `json:"at,omitzero"`
}

// Choice is something the owner can pick to fill the form: a feed a page advertises, or a tag a server lists.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
}

// Preview is what a look-up found. Target and Instance are the cleaned-up values a save would keep. Problem says in plain words why nothing was found.
type Preview struct {
	Found    bool     `json:"found"`
	Target   string   `json:"target,omitempty"`
	Instance string   `json:"instance,omitempty"`
	Title    string   `json:"title,omitempty"`
	Sample   []Sample `json:"sample"`
	Choices  []Choice `json:"choices"`
	Problem  string   `json:"problem,omitempty"`

	transient bool
}

// Lookup reads one source once, without saving anything, and says what it found. The value is reduced and checked first; one that fails its check is
// a problem with no request. A Telegram look-up has its own reader, so it never takes the timer's politeness slot (the caller keeps its own floor).
func Lookup(ctx context.Context, f stacksdk.Fetch, kind, target, instance string) Preview {
	switch kind {
	case "telegram":
		return lookupTelegram(ctx, f, target)
	case "mastodon":
		return lookupMastodon(ctx, f, target, instance)
	case "hackernews":
		return lookupHN(ctx, f, target)
	case "feed":
		return lookupFeed(ctx, f, target)
	case "page":
		return lookupPage(ctx, f, target)
	}
	return Preview{Problem: "this kind of source cannot be looked up"}
}

func problem(words string) Preview { return Preview{Problem: words} }

// unread is the preview of a source that could not be read. A switched-off internet or a slow answer is transient: it says nothing about the source, so a
// caller must not remember it.
func unread(kind string, err error) Preview {
	code := stacksdk.CodeOf(err)
	return Preview{Problem: whyNotRead(kind, err), transient: code == stacksdk.CodeUnavailable || code == stacksdk.CodeTimeout}
}

// Transient reports that the problem is about the moment (internet off, no answer in time), not about the value looked up.
func (p Preview) Transient() bool { return p.transient }

func lookupTelegram(ctx context.Context, f stacksdk.Fetch, target string) Preview {
	name := ChannelName(target)
	if err := CheckChannel(name); err != nil {
		return problem(err.Error())
	}
	body, err := get(ctx, f, name, "https://t.me/s/"+name, "text/html")
	if err != nil {
		return unread("telegram", err)
	}
	items, err := ParseTelegram(name, name, body)
	if err != nil {
		return unread("telegram", err)
	}
	return Preview{Found: true, Target: name, Title: firstNonEmpty(channelTitle(body), name), Sample: samples(items)}
}

func lookupMastodon(ctx context.Context, f stacksdk.Fetch, target, instance string) Preview {
	tag, host := TagName(target), InstanceName(instance)
	if err := CheckTag(tag); err != nil {
		return problem(err.Error())
	}
	if err := CheckInstance(host); err != nil {
		return problem(err.Error())
	}
	items, err := MastodonTag{Label: tag, Instance: host, Tag: tag, Fetcher: f}.Fetch(ctx)
	if err != nil {
		return unread("mastodon", err)
	}
	return Preview{Found: true, Target: tag, Instance: host, Title: "#" + tag + " on " + host, Sample: samples(items)}
}

func lookupHN(ctx context.Context, f stacksdk.Fetch, target string) Preview {
	q := strings.TrimSpace(target)
	if err := CheckQuery(q); err != nil {
		return problem(err.Error())
	}
	items, err := HackerNews{Label: q, Query: q, Fetcher: f}.Fetch(ctx)
	if err != nil {
		return unread("hackernews", err)
	}
	return Preview{Found: true, Target: q, Title: "Hacker News: " + q, Sample: samples(items)}
}

func lookupFeed(ctx context.Context, f stacksdk.Fetch, target string) Preview {
	addr := strings.TrimSpace(target)
	if err := CheckAddress(addr); err != nil {
		return problem(err.Error())
	}
	body, err := get(ctx, f, addr, addr, "application/atom+xml, application/rss+xml, application/xml, text/xml, text/html")
	if err != nil {
		return unread("feed", err)
	}
	items, err := ParseFeed(addr, body)
	if err != nil {
		// Not a feed: the address may be a site that advertises one.
		found := AdvertisedFeeds(addr, body)
		words := "That address is not a feed and does not name one. Try the site's feed address, often ending in /feed or /rss."
		if len(found) > 0 {
			words = "That address is not a feed itself, but the page offers these. Pick one."
		}
		return Preview{Target: addr, Choices: found, Problem: words}
	}
	return Preview{Found: true, Target: addr, Title: firstNonEmpty(FeedTitle(body), addr), Sample: samples(items)}
}

func lookupPage(ctx context.Context, f stacksdk.Fetch, target string) Preview {
	addr := strings.TrimSpace(target)
	if err := CheckAddress(addr); err != nil {
		return problem(err.Error())
	}
	body, err := get(ctx, f, addr, addr, "text/html, application/xhtml+xml")
	if err != nil {
		return unread("page", err)
	}
	item, err := PageItem(addr, addr, body)
	if err != nil {
		return unread("page", err)
	}
	title := firstNonEmpty(PageTitle(body), addr)
	return Preview{Found: true, Target: addr, Title: title, Sample: []Sample{{Title: title, Link: item.Link, Text: clip(item.Text, 200)}}}
}

func samples(items []Item) []Sample {
	out := make([]Sample, 0, SampleSize)
	for _, it := range items {
		if len(out) == SampleSize {
			break
		}
		out = append(out, Sample{Title: it.Title, Link: it.Link, At: it.At})
	}
	return out
}

// whyNotRead words a failed read for the owner. The words are fixed sentences: nothing the remote side sent is repeated back.
func whyNotRead(kind string, err error) string {
	switch stacksdk.CodeOf(err) {
	case stacksdk.CodeUnavailable:
		return "Internet access is off, so nothing can be looked up. Switch it on under Guardrails."
	case stacksdk.CodeTimeout:
		return "The server did not answer in time. Try again in a moment."
	}
	if kind == "telegram" {
		return "No public preview was found for that channel. It may not exist, or it may not allow a public preview."
	}
	if errors.Is(err, ErrShape) {
		return "It answered, but not in a form this reader understands."
	}
	return "It could not be read. Check the spelling, or try again later."
}

var (
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	ogTitleRe = regexp.MustCompile(`(?is)<meta[^>]+property="og:title"[^>]+content="([^"]*)"`)
	linkTagRe = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attrRe    = regexp.MustCompile(`(?is)([a-z][a-z-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// channelTitle is a Telegram channel's display name from its preview page.
func channelTitle(body []byte) string {
	if m := ogTitleRe.FindSubmatch(body); m != nil {
		return Line(html.UnescapeString(string(m[1])), MaxTitleRune)
	}
	return ""
}

// PageTitle is the text of a web page's <title>, or empty.
func PageTitle(body []byte) string {
	if m := titleRe.FindSubmatch(body); m != nil {
		return Line(html.UnescapeString(string(m[1])), MaxTitleRune)
	}
	return ""
}

// FeedTitle is the title of an RSS or Atom document itself, or empty.
func FeedTitle(body []byte) string {
	var doc struct {
		Title   string `xml:"title"`
		Channel struct {
			Title string `xml:"title"`
		} `xml:"channel"`
	}
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.Strict = false
	_ = dec.Decode(&doc)
	return Line(firstNonEmpty(doc.Channel.Title, doc.Title), MaxTitleRune)
}

// AdvertisedFeeds lists the RSS and Atom feeds an HTML page names in its head, as full https addresses. A relative address is resolved against the page's
// own; one that fails CheckAddress (plain http, no host) is dropped, as are repeats.
func AdvertisedFeeds(base string, body []byte) []Choice {
	root, err := url.Parse(base)
	if err != nil {
		return nil
	}
	out := []Choice{}
	seen := map[string]bool{}
	for _, tag := range linkTagRe.FindAll(body, 40) {
		attrs := map[string]string{}
		for _, m := range attrRe.FindAllStringSubmatch(string(tag), -1) {
			attrs[strings.ToLower(m[1])] = html.UnescapeString(firstNonEmpty(m[2], m[3]))
		}
		kind := strings.ToLower(attrs["type"])
		if !strings.EqualFold(attrs["rel"], "alternate") || (kind != "application/rss+xml" && kind != "application/atom+xml") {
			continue
		}
		ref, err := url.Parse(attrs["href"])
		if err != nil {
			continue
		}
		full := root.ResolveReference(ref).String()
		if CheckAddress(full) != nil || seen[full] {
			continue
		}
		seen[full] = true
		out = append(out, Choice{Value: full, Label: firstNonEmpty(Line(attrs["title"], MaxTitleRune), full)})
	}
	return out
}

// Suggest lists what the owner can pick while typing. Only Mastodon has a keyless list (a tag search, or the server's trending tags when nothing is typed);
// every other kind answers nothing and no problem, because a Telegram channel search needs a login and a Hacker News search is previewed by Lookup. The
// problem is set only when a list was asked for and cannot be given.
func Suggest(ctx context.Context, f stacksdk.Fetch, kind, instance, q string) ([]Choice, string) {
	if kind != "mastodon" {
		return nil, ""
	}
	host := InstanceName(instance)
	if err := CheckInstance(host); err != nil {
		return nil, err.Error()
	}
	typed := TagName(q)
	target := fmt.Sprintf("https://%s/api/v1/trends/tags?limit=%d", host, suggestLimit)
	if typed != "" {
		if err := CheckTag(typed); err != nil {
			return nil, ""
		}
		target = fmt.Sprintf("https://%s/api/v2/search?type=hashtags&limit=%d&q=%s", host, suggestLimit, url.QueryEscape(typed))
	}
	body, err := get(ctx, f, host, target, "application/json")
	if err != nil {
		return nil, whyNotRead("mastodon", err)
	}
	choices, err := parseTags(body, typed != "")
	if err != nil {
		return nil, whyNotRead("mastodon", err)
	}
	if len(choices) == 0 && typed == "" {
		return nil, host + " shares no trending tags. Type a tag to search."
	}
	return choices, ""
}

// parseTags reads a tag list: the trends answer is the list itself, a search answer holds it under "hashtags".
func parseTags(body []byte, search bool) ([]Choice, error) {
	type tag struct {
		Name    string `json:"name"`
		History []struct {
			Uses string `json:"uses"`
		} `json:"history"`
	}
	var tags []tag
	if search {
		var doc struct {
			Hashtags []tag `json:"hashtags"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, ErrShape
		}
		tags = doc.Hashtags
	} else if err := json.Unmarshal(body, &tags); err != nil {
		return nil, ErrShape
	}
	out := []Choice{}
	for _, t := range tags {
		if CheckTag(t.Name) != nil {
			continue
		}
		note, used := "", 0
		for _, h := range t.History {
			var n int
			_, _ = fmt.Sscanf(h.Uses, "%d", &n)
			used += n
		}
		if used > 0 {
			note = fmt.Sprintf("%d posts this week", used)
		}
		out = append(out, Choice{Value: t.Name, Label: "#" + t.Name, Note: note})
	}
	return out, nil
}
