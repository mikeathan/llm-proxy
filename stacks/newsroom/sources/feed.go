package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"

	"llm-proxy-stacks/sdk"
)

// Feed reads an RSS 2.0 or Atom feed: any site's feed, and a GitHub repository's releases.atom.
type Feed struct {
	Label   string
	URL     string
	Fetcher stacksdk.Fetch
}

// Name implements Source.
func (f Feed) Name() string { return f.Label }

// Fetch implements Source.
func (f Feed) Fetch(ctx context.Context) ([]Item, error) {
	body, err := get(ctx, f.Fetcher, f.Label, f.URL, "application/atom+xml, application/rss+xml, application/xml, text/xml")
	if err != nil {
		return nil, err
	}
	items, err := ParseFeed(f.Label, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Label, err)
	}
	return items, nil
}

type rssDoc struct {
	Items []struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		GUID        string `xml:"guid"`
		PubDate     string `xml:"pubDate"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
}

type atomDoc struct {
	Entries []struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Publish string `xml:"published"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
		Links   []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// ParseFeed reads feed bytes. The root element decides the dialect; anything that is not an rss or feed document is ErrShape.
func ParseFeed(source string, body []byte) ([]Item, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	root := ""
	for root == "" {
		tok, err := dec.Token()
		if err != nil {
			return nil, ErrShape
		}
		if se, ok := tok.(xml.StartElement); ok {
			root = se.Name.Local
		}
	}
	// Decode again from the start with the dialect's own shape.
	switch root {
	case "rss":
		return parseRSS(source, body)
	case "feed":
		return parseAtom(source, body)
	}
	return nil, ErrShape
}

func parseRSS(source string, body []byte) ([]Item, error) {
	var doc rssDoc
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&doc); err != nil && err != io.EOF {
		return nil, ErrShape
	}
	var out []Item
	for _, it := range doc.Items {
		out = append(out, Item{
			Source: source, Key: Line(it.GUID, 200), Title: Line(it.Title, MaxTitleRune), Link: CanonicalLink(it.Link),
			Text: Plain(it.Description, MaxTextRune), At: parseTime(it.PubDate),
		})
	}
	return bound(out), nil
}

func parseAtom(source string, body []byte) ([]Item, error) {
	var doc atomDoc
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&doc); err != nil && err != io.EOF {
		return nil, ErrShape
	}
	var out []Item
	for _, e := range doc.Entries {
		link := ""
		for _, l := range e.Links {
			if l.Rel == "alternate" || l.Rel == "" {
				link = CanonicalLink(l.Href)
				break
			}
		}
		text := e.Summary
		if text == "" {
			text = e.Content
		}
		when := e.Updated
		if e.Publish != "" {
			when = e.Publish
		}
		out = append(out, Item{
			Source: source, Key: Line(e.ID, 200), Title: Line(e.Title, MaxTitleRune), Link: link,
			Text: Plain(text, MaxTextRune), At: parseTime(when),
		})
	}
	return bound(out), nil
}
