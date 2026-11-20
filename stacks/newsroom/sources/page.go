package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"llm-proxy-stacks/sdk"
)

// Page watches one web page for change: it fetches the page, reduces it to its visible text and reports one item whose Key is the hash of that text.
// The seen ledger sees a new Key when the text changed and nothing when it did not. Markup-only changes (scripts, styles, attributes) do not count.
type Page struct {
	Label   string
	URL     string
	Fetcher stacksdk.Fetch
}

// Name implements Source.
func (p Page) Name() string { return p.Label }

// Fetch implements Source.
func (p Page) Fetch(ctx context.Context) ([]Item, error) {
	body, err := get(ctx, p.Fetcher, p.Label, p.URL, "text/html, application/xhtml+xml")
	if err != nil {
		return nil, err
	}
	item, err := PageItem(p.Label, p.URL, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.Label, err)
	}
	return []Item{item}, nil
}

// PageItem builds the single item for a page. An empty page is ErrShape: a blank answer is a login wall or a failure, not "the page is empty".
func PageItem(source, link string, body []byte) (Item, error) {
	text := Plain(string(body), 0)
	if text == "" {
		return Item{}, ErrShape
	}
	sum := sha256.Sum256([]byte(text))
	return Item{
		Source: source, Key: hex.EncodeToString(sum[:8]), Version: hex.EncodeToString(sum[:8]), Title: Line(source, MaxTitleRune), Link: CanonicalLink(link),
		Text: clip(text, MaxTextRune),
	}, nil
}
