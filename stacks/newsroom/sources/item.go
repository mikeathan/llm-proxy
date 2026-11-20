// Package sources reads things the Newsroom follows: feeds, searches, public channels and pages. Each adapter takes the SDK's guarded
// Fetch port, makes plain GETs and returns typed Items; it never calls a model, never searches the web and never decides whether an item is new
// (that is the seen ledger and the delta step, later phases). Everything it returns is untrusted text from the internet (Constitution V.2): cut to
// a bound, stripped of markup and control characters, and never an instruction.
package sources

import (
	"context"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Bounds on what one fetch may return, so a hostile or runaway source cannot grow memory, a store or a prompt without limit.
const (
	MaxItems     = 50
	MaxTitleRune = 200
	MaxTextRune  = 1000
	// maxBody is the most one request reads; the core's port applies its own ceiling as well.
	maxBody = 1 << 20
)

// Item is one thing a source reported. Link is canonical (see CanonicalLink) and is the identity the seen ledger keys on; Key is the source's own id
// when it has one (a tag, a post id) and is used only when an item has no link.
type Item struct {
	Source string `json:"source"`
	Key    string `json:"key,omitempty"`
	Title  string `json:"title"`
	Link   string `json:"link,omitempty"`
	Text   string `json:"text,omitempty"`
	// Version changes when the content behind the same Link changed (a watched page's text hash); empty for a post or an article, whose identity
	// is its Link alone.
	Version string    `json:"version,omitempty"`
	At      time.Time `json:"at,omitzero"`
}

// Identity is what the seen ledger keys on: the canonical link (plus the content version for a watched page), or the source's own id when the
// item has no link. Two items with one identity are the same thing.
func (i Item) Identity() string {
	if i.Link != "" {
		return i.Link + "\x00" + i.Version
	}
	return "key:" + i.Source + "\x00" + i.Key
}

// Source is one configured place to read. Name is the owner's label, shown on the row and kept in Item.Source.
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]Item, error)
}

// ErrShape is returned when a source answered but not in the shape the adapter reads (a page redesign, a login wall, a channel with no public
// preview). It must be shown as "source failing", never as "nothing new".
var ErrShape = errors.New("the source answered in a shape this reader does not understand")

// ErrTooSoon is returned by a source with a politeness floor when it is asked again before the floor has passed.
var ErrTooSoon = errors.New("the source was read too recently; it has a minimum gap between reads")

var (
	tagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
	blockEndRe = regexp.MustCompile(`(?i)<\s*(?:br\s*/?|/p|/div|/li)\s*>`)
	dropRe     = regexp.MustCompile(`(?is)<(script|style|svg|noscript)\b.*?</(?:script|style|svg|noscript)\s*>`)
	spaceRe    = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	blankRe    = regexp.MustCompile(`\n\s*\n+`)
	trackingRe = regexp.MustCompile(`^(utm_.*|fbclid|gclid|ref_src)$`)
)

// Plain turns markup into bounded plain text: scripts and styles go, block ends become line breaks, tags go, entities are decoded, control characters
// are removed and the result is cut to max runes. A cut ends with an ellipsis.
func Plain(markup string, max int) string {
	s := dropRe.ReplaceAllString(markup, " ")
	s = blockEndRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return ' '
	}, s)
	s = spaceRe.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = blankRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return clip(strings.TrimSpace(s), max)
}

// Line is Plain on a single line: for titles.
func Line(markup string, max int) string {
	return Plain(strings.Join(strings.Fields(Plain(markup, 0)), " "), max)
}

func clip(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// CanonicalLink normalises an http(s) link so the same page under a tracking parameter, a fragment, a leading "www." or a trailing slash has one key. It
// returns "" for anything that is not an http or https URL with a host, so a javascript: or data: link never becomes an item's identity. It matches the
// automation seen ledger's key (backend automation.canonicalURL) for such links, so an item can be looked up in the same ledger; a stack cannot import
// that function, so the rule is repeated here and pinned by a test.
func CanonicalLink(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), ".,;")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	q := u.Query()
	for k := range q {
		if trackingRe.MatchString(strings.ToLower(k)) {
			q.Del(k)
		}
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	out := strings.ToLower(u.Scheme) + "://" + host + strings.TrimRight(u.EscapedPath(), "/")
	if enc := q.Encode(); enc != "" {
		out += "?" + enc
	}
	return out
}

// bound keeps at most MaxItems items.
func bound(items []Item) []Item {
	if len(items) > MaxItems {
		return items[:MaxItems]
	}
	return items
}

// parseTime reads the layouts the sources use; the zero time means "unknown".
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
