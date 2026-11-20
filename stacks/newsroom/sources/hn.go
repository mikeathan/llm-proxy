package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"llm-proxy-stacks/sdk"
)

// HackerNews searches Hacker News stories, newest first, through the keyless Algolia API.
type HackerNews struct {
	Label   string
	Query   string
	Fetcher stacksdk.Fetch
}

// Name implements Source.
func (h HackerNews) Name() string { return h.Label }

// Fetch implements Source.
func (h HackerNews) Fetch(ctx context.Context) ([]Item, error) {
	target := "https://hn.algolia.com/api/v1/search_by_date?tags=story&hitsPerPage=30&query=" + url.QueryEscape(h.Query)
	body, err := get(ctx, h.Fetcher, h.Label, target, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := ParseHN(h.Label, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.Label, err)
	}
	return items, nil
}

// ParseHN reads an Algolia search_by_date answer. A hit with no external url is a text post: its identity is its Hacker News page.
func ParseHN(source string, body []byte) ([]Item, error) {
	var doc struct {
		Hits *[]struct {
			ObjectID  string `json:"objectID"`
			Title     string `json:"title"`
			URL       string `json:"url"`
			StoryText string `json:"story_text"`
			CreatedAt string `json:"created_at"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || doc.Hits == nil {
		return nil, ErrShape
	}
	var out []Item
	for _, h := range *doc.Hits {
		link := CanonicalLink(h.URL)
		if link == "" && h.ObjectID != "" {
			link = CanonicalLink("https://news.ycombinator.com/item?id=" + url.QueryEscape(h.ObjectID))
		}
		out = append(out, Item{
			Source: source, Key: Line(h.ObjectID, 40), Title: Line(h.Title, MaxTitleRune), Link: link,
			Text: Plain(h.StoryText, MaxTextRune), At: parseTime(h.CreatedAt),
		})
	}
	return bound(out), nil
}
