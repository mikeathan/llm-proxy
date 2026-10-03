package automation

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"llm-proxy/models"
)

const (
	// maxTitleRunes bounds a stored title. Titles come from web-derived model
	// output and are later put back into prompts, so they are kept short and
	// single-line.
	maxTitleRunes = 100
	// maxSeenEntries bounds a seen ledger so a runaway report cannot grow the
	// file (and the prompt hint source) without limit; the oldest entries go.
	maxSeenEntries = 2000
)

// noNewItemsMessage is sent in place of an empty digest when NotifyConfig.SendEmpty
// is set and every row was already reported.
const noNewItemsMessage = "No new items since the last run."

var (
	urlPattern = regexp.MustCompile(`https?://[^\s)|>\]]+`)
	mdLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdNoiseRe  = regexp.MustCompile("[*`]")
	// bulletMarkerRe matches a list marker (-, *, •, 1. or 1)) after any indent.
	bulletMarkerRe = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s+`)
	// trackingKey matches pure tracking parameters only; "ref" is deliberately
	// absent because code hosts use it to select a different page (?ref=main).
	trackingKey = regexp.MustCompile(`^(utm_.*|fbclid|gclid|ref_src)$`)
)

// digest is the chat-ready form of a report plus the items to remember once it
// has actually been delivered.
type digest struct {
	// Message is the text to send; empty means there is nothing to deliver.
	Message string
	// NewItems maps canonical URL -> title for rows first reported by this digest.
	NewItems map[string]string
}

// buildDigest turns an agent report into a chat message. Markdown tables become
// bullet lists (chat clients do not render tables); when cfg.Dedup is set, table
// rows and link bullets whose link is in the ledger (and younger than the
// retention window) are dropped. Items without a link cannot be identified, so
// they are always kept.
func buildDigest(report string, ledger models.SeenLedger, cfg models.NotifyConfig, now time.Time) digest {
	b := digestBuilder{
		cfg:      cfg,
		ledger:   ledger,
		cutoff:   now.AddDate(0, 0, -cfg.RetentionDays()),
		reported: map[string]bool{},
		d:        digest{NewItems: map[string]string{}},
	}
	lines := strings.Split(strings.TrimSpace(report), "\n")
	for i := 0; i < len(lines); {
		if end := tableEnd(lines, i); end > i {
			b.addTable(lines[i+2 : end])
			i = end
			continue
		}
		i = b.addLine(lines, i)
	}
	return b.finish()
}

// digestBuilder accumulates the chat message and the deduplication state that
// table rows and link bullets share.
type digestBuilder struct {
	cfg      models.NotifyConfig
	ledger   models.SeenLedger
	cutoff   time.Time
	reported map[string]bool // keys already seen in this report
	d        digest
	out      []string
	items    int // link-bearing items found (rows and bullets)
	kept     int
}

// admit reports whether an item is kept. With dedup on, an item whose link is in
// the ledger (inside the retention window) or already appeared in this report is
// dropped; a kept item with a link is remembered as new.
func (b *digestBuilder) admit(item tableRow) bool {
	b.items++
	if b.cfg.Dedup && item.key != "" {
		if e, seen := b.ledger[item.key]; (seen && e.At.After(b.cutoff)) || b.reported[item.key] {
			return false
		}
		b.reported[item.key] = true
		b.d.NewItems[item.key] = clipTitle(item.title)
	}
	b.kept++
	return true
}

func (b *digestBuilder) addTable(rows []string) {
	for _, line := range rows {
		if row := parseRow(line); b.admit(row) {
			b.out = append(b.out, row.render())
		}
	}
}

// addLine handles a non-table line at lines[i] and returns the index of the next
// unprocessed line. A bullet with a link is one item: when it is dropped, its
// more-deeply indented detail lines go with it.
func (b *digestBuilder) addLine(lines []string, i int) int {
	line := lines[i]
	item, ok := parseBullet(line)
	if !ok {
		b.out = append(b.out, line)
		return i + 1
	}
	keep := b.admit(item)
	if keep {
		b.out = append(b.out, line)
	}
	indent := indentOf(line)
	i++
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" && indentOf(lines[i]) > indent {
		// Under a kept bullet, a nested link bullet is an item in its own right:
		// judged and remembered like any other, so it is not re-sent next run.
		// Under a dropped bullet everything nested goes with it.
		if _, nested := parseBullet(lines[i]); keep && nested {
			i = b.addLine(lines, i)
			continue
		}
		if keep {
			b.out = append(b.out, lines[i])
		}
		i++
	}
	return i
}

// finish returns the digest. A report whose every item was already reported is
// not delivered (or becomes the "no new items" line when SendEmpty is set).
func (b *digestBuilder) finish() digest {
	if b.items > 0 && b.kept == 0 {
		if b.cfg.SendEmpty {
			b.d.Message = noNewItemsMessage
		}
		return b.d
	}
	b.d.Message = collapseBlankLines(strings.Join(b.out, "\n"))
	return b.d
}

// tableEnd returns the exclusive end of a markdown table starting at lines[i]
// (header, separator, rows), or i when no table starts there.
func tableEnd(lines []string, i int) int {
	if i+1 >= len(lines) || !isTableLine(lines[i]) || !isSeparator(lines[i+1]) {
		return i
	}
	end := i + 2
	for end < len(lines) && isTableLine(lines[end]) {
		end++
	}
	return end
}

func isTableLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

func isSeparator(line string) bool {
	if !isTableLine(line) {
		return false
	}
	rest := strings.Trim(strings.NewReplacer("|", "", ":", "", " ", "", "\t", "").Replace(line), "-")
	return rest == "" && strings.Contains(line, "-")
}

type tableRow struct {
	key   string // canonical URL, "" when the row has no link
	title string
	rest  string
	url   string
}

func parseRow(line string) tableRow {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(trimmed, "|")
	row := tableRow{}
	if m := urlPattern.FindString(trimmed); m != "" {
		row.url = strings.TrimRight(m, ".,;")
		row.key = canonicalURL(row.url)
	}
	var rest []string
	for i, c := range cells {
		text := cleanCell(c)
		switch {
		case i == 0:
			row.title = text
		case text != "" && !urlPattern.MatchString(c):
			rest = append(rest, text)
		}
	}
	row.rest = strings.Join(rest, " · ")
	return row
}

// parseBullet recognises a markdown bullet or numbered item that carries a link
// and identifies it by that link, like a table row. The title is the item's text
// with link markup removed.
func parseBullet(line string) (tableRow, bool) {
	loc := bulletMarkerRe.FindStringIndex(line)
	if loc == nil {
		return tableRow{}, false
	}
	text := line[loc[1]:]
	m := urlPattern.FindString(text)
	if m == "" {
		return tableRow{}, false
	}
	row := tableRow{url: strings.TrimRight(m, ".,;")}
	row.key = canonicalURL(row.url)
	row.title = cleanCell(urlPattern.ReplaceAllString(mdLinkRe.ReplaceAllString(text, "$1"), ""))
	return row, true
}

// indentOf counts a line's leading whitespace; a tab counts as one column.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func (r tableRow) render() string {
	var b strings.Builder
	b.WriteString("• " + r.title)
	if r.rest != "" {
		b.WriteString(" — " + r.rest)
	}
	if r.url != "" {
		b.WriteString("\n  " + r.url)
	}
	return b.String()
}

// clipTitle makes a title safe to store and to put back into a prompt: control
// characters dropped, whitespace collapsed to single spaces, length bounded.
func clipTitle(title string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)
	clean = strings.Join(strings.Fields(clean), " ")
	if r := []rune(clean); len(r) > maxTitleRunes {
		clean = string(r[:maxTitleRunes])
	}
	return clean
}

func cleanCell(c string) string {
	c = mdLinkRe.ReplaceAllString(c, "$1")
	return strings.TrimSpace(mdNoiseRe.ReplaceAllString(c, ""))
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// canonicalURL normalises a link so the same article reported via different
// tracking parameters, fragments or a trailing slash is recognised as seen.
func canonicalURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), ".,;")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	q := u.Query()
	for k := range q {
		if trackingKey.MatchString(strings.ToLower(k)) {
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

// mergeSeen returns the ledger with items added at now and entries older than
// retentionDays removed. The input map is not modified.
func mergeSeen(ledger models.SeenLedger, items map[string]string, now time.Time, retentionDays int) models.SeenLedger {
	cutoff := now.AddDate(0, 0, -retentionDays)
	merged := make(models.SeenLedger, len(ledger)+len(items))
	for k, e := range ledger {
		if e.At.After(cutoff) {
			merged[k] = e
		}
	}
	for k, title := range items {
		merged[k] = models.SeenEntry{Title: title, At: now}
	}
	return capLedger(merged)
}

// capLedger drops the oldest entries beyond maxSeenEntries.
func capLedger(ledger models.SeenLedger) models.SeenLedger {
	if len(ledger) <= maxSeenEntries {
		return ledger
	}
	keys := make([]string, 0, len(ledger))
	for k := range ledger {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return ledger[keys[i]].At.After(ledger[keys[j]].At) })
	for _, k := range keys[maxSeenEntries:] {
		delete(ledger, k)
	}
	return ledger
}

// recentTitles lists up to limit distinct titles still inside the retention
// window, newest first. It feeds the "already reported" block of the prompt.
func recentTitles(ledger models.SeenLedger, now time.Time, retentionDays, limit int) []string {
	cutoff := now.AddDate(0, 0, -retentionDays)
	entries := make([]models.SeenEntry, 0, len(ledger))
	for _, e := range ledger {
		if e.At.After(cutoff) && e.Title != "" {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].At.After(entries[j].At) })

	seen := map[string]bool{}
	var titles []string
	for _, e := range entries {
		if len(titles) == limit {
			break
		}
		if !seen[e.Title] {
			seen[e.Title] = true
			titles = append(titles, e.Title)
		}
	}
	return titles
}
