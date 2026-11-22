package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"llm-proxy-stacks/sdk"
)

// The news search reads Google News' keyless RSS search (no account, no key). It is an aggregator: one query reaches many publishers, Reuters and AP
// among them, which have no feed of their own. What it gives is thin: a headline, a publisher and a time, and a link that is a Google page rather than
// the article. It is unofficial and Google can change or refuse it; its notice allows personal, non-commercial use in a feed reader.
const (
	newsSearchBase = "https://news.google.com/rss/search"
	// DefaultNewsLocale is the language and country of a search that names none: Google's edition for it.
	DefaultNewsLocale = "en-US"
	publisherSep      = " - "
)

// NewsSearchURL is the address of a query's feed for a locale such as "en-GB" (language-COUNTRY). The query is encoded, never spliced in.
func NewsSearchURL(query, locale string) string {
	if locale == "" {
		locale = DefaultNewsLocale
	}
	lang, country, _ := strings.Cut(locale, "-")
	v := url.Values{"q": {query}, "hl": {locale}, "gl": {country}, "ceid": {country + ":" + lang}}
	return newsSearchBase + "?" + v.Encode()
}

// NewsSearch reads the news matching a query. Every item is headline only: Text is empty, because the feed's description repeats the title and a
// summary written from a bare headline would be invention. The publisher comes out of the title (Google appends " - Publisher") into Item.Publisher.
type NewsSearch struct {
	Label   string
	Query   string
	Locale  string
	Fetcher stacksdk.Fetch
}

// Name implements Source.
func (n NewsSearch) Name() string { return n.Label }

// Fetch implements Source.
func (n NewsSearch) Fetch(ctx context.Context) ([]Item, error) {
	body, err := get(ctx, n.Fetcher, n.Label, NewsSearchURL(n.Query, n.Locale), "application/rss+xml, application/xml, text/xml")
	if err != nil {
		return nil, err
	}
	items, err := ParseFeed(n.Label, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", n.Label, err)
	}
	for i := range items {
		items[i].Text = ""
		items[i].Title = strings.TrimSuffix(items[i].Title, publisherSep+items[i].Publisher)
	}
	return items, nil
}
