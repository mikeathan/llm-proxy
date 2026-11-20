package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"llm-proxy-stacks/sdk"
)

// instanceRe accepts a plain host name: no scheme, port, path or user info, so a configured instance cannot redirect a request elsewhere.
var instanceRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// MastodonTag reads the public timeline of one hashtag on one instance.
type MastodonTag struct {
	Label    string
	Instance string
	Tag      string
	Fetcher  stacksdk.Fetch
}

// Name implements Source.
func (m MastodonTag) Name() string { return m.Label }

// Fetch implements Source.
func (m MastodonTag) Fetch(ctx context.Context) ([]Item, error) {
	if !instanceRe.MatchString(m.Instance) {
		return nil, fmt.Errorf("%s: the instance must be a host name such as mastodon.social", m.Label)
	}
	target := "https://" + m.Instance + "/api/v1/timelines/tag/" + url.PathEscape(m.Tag) + "?limit=30"
	body, err := get(ctx, m.Fetcher, m.Label, target, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := ParseMastodon(m.Label, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Label, err)
	}
	return items, nil
}

// ParseMastodon reads a tag timeline. A post has no title, so the title is the first line of its text.
func ParseMastodon(source string, body []byte) ([]Item, error) {
	var posts []struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		URL       string `json:"url"`
		Content   string `json:"content"`
		Spoiler   string `json:"spoiler_text"`
		Account   struct {
			Acct string `json:"acct"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, ErrShape
	}
	var out []Item
	for _, p := range posts {
		text := Plain(p.Content, MaxTextRune)
		out = append(out, Item{
			Source: source, Key: Line(p.ID, 40), Title: Line(firstNonEmpty(p.Spoiler, text), MaxTitleRune), Link: CanonicalLink(p.URL),
			Text: text, At: parseTime(p.CreatedAt),
		})
	}
	return bound(out), nil
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if p != "" {
			return p
		}
	}
	return ""
}
