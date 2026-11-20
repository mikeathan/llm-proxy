package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// DefaultTelegramGap is the least time between two reads of one channel page. The page is a web preview, not an API (owner decision D3, 2026-11-20):
// it is read politely, a channel at a time, and only for channels the owner listed.
const DefaultTelegramGap = 10 * time.Minute

var (
	channelRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)
	postRe     = regexp.MustCompile(`data-post="([A-Za-z][A-Za-z0-9_]{3,31})/(\d+)"`)
	textRe     = regexp.MustCompile(`(?s)class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>`)
	datetimeRe = regexp.MustCompile(`<time[^>]*datetime="([^"]+)"`)
)

// TelegramChannel reads the public web preview of one channel (https://t.me/s/<channel>). No login, no Bot API: only a channel that has a public
// preview works, and one without it answers ErrShape.
type TelegramChannel struct {
	Label   string
	Channel string
	Fetcher stacksdk.Fetch
	// Gap is the least time between reads; zero means DefaultTelegramGap.
	Gap time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	mu   sync.Mutex
	last time.Time
}

// Name implements Source.
func (t *TelegramChannel) Name() string { return t.Label }

// Fetch implements Source.
func (t *TelegramChannel) Fetch(ctx context.Context) ([]Item, error) {
	if !channelRe.MatchString(t.Channel) {
		return nil, fmt.Errorf("%s: a channel name is 4 to 32 letters, digits or underscores, without the @", t.Label)
	}
	if err := t.reserve(); err != nil {
		return nil, fmt.Errorf("%s: %w", t.Label, err)
	}
	body, err := get(ctx, t.Fetcher, t.Label, "https://t.me/s/"+t.Channel, "text/html")
	if err != nil {
		return nil, err
	}
	items, err := ParseTelegram(t.Label, t.Channel, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Label, err)
	}
	return items, nil
}

// reserve takes the next read slot, or says it is too soon. A failed read still uses its slot: hammering a failing page is what the floor prevents.
func (t *TelegramChannel) reserve() error {
	gap, now := t.Gap, time.Now
	if gap <= 0 {
		gap = DefaultTelegramGap
	}
	if t.Now != nil {
		now = t.Now
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at := now()
	if !t.last.IsZero() && at.Sub(t.last) < gap {
		return ErrTooSoon
	}
	t.last = at
	return nil
}

// ParseTelegram reads a channel preview page, newest message last as the page lists them, and returns them newest first. Only messages of the named
// channel count (a preview can show a forwarded post). A page with no message is ErrShape. A message with no text (a photo alone) is skipped.
func ParseTelegram(source, channel string, body []byte) ([]Item, error) {
	parts := strings.Split(string(body), "tgme_widget_message_wrap")
	if len(parts) < 2 {
		return nil, ErrShape
	}
	var out []Item
	for _, part := range parts[1:] {
		if item, ok := telegramMessage(source, channel, part); ok {
			out = append([]Item{item}, out...)
		}
	}
	if len(out) == 0 {
		return nil, ErrShape
	}
	return bound(out), nil
}

func telegramMessage(source, channel, part string) (Item, bool) {
	post := postRe.FindStringSubmatch(part)
	text := textRe.FindStringSubmatch(part)
	if post == nil || text == nil || !strings.EqualFold(post[1], channel) {
		return Item{}, false
	}
	plain := Plain(text[1], MaxTextRune)
	if plain == "" {
		return Item{}, false
	}
	item := Item{
		Source: source, Key: post[1] + "/" + post[2], Title: Line(plain, MaxTitleRune),
		Link: "https://t.me/" + post[1] + "/" + post[2], Text: plain,
	}
	if when := datetimeRe.FindStringSubmatch(part); when != nil {
		item.At = parseTime(when[1])
	}
	return item, true
}
