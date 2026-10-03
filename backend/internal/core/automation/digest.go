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
// bullet lists (chat clients do not render tables); when cfg.Dedup is set, rows
// whose link is in the ledger (and younger than the retention window) are
// dropped. Rows without a link cannot be identified, so they are always kept.
func buildDigest(report string, ledger models.SeenLedger, cfg models.NotifyConfig, now time.Time) digest {
	d := digest{NewItems: map[string]string{}}
	lines := strings.Split(strings.TrimSpace(report), "\n")
	cutoff := now.AddDate(0, 0, -cfg.RetentionDays())

	var out []string
	rows, kept := 0, 0
	reported := map[string]bool{}
	for i := 0; i < len(lines); {
		end := tableEnd(lines, i)
		if end == i {
			out = append(out, lines[i])
			i++
			continue
		}
		for _, line := range lines[i+2 : end] {
			rows++
			row := parseRow(line)
			if cfg.Dedup && row.key != "" {
				if e, seen := ledger[row.key]; (seen && e.At.After(cutoff)) || reported[row.key] {
					continue
				}
				reported[row.key] = true
				d.NewItems[row.key] = clipTitle(row.title)
			}
			kept++
			out = append(out, row.render())
		}
		i = end
	}

	if rows > 0 && kept == 0 {
		if cfg.SendEmpty {
			d.Message = noNewItemsMessage
		}
		return d
	}
	d.Message = collapseBlankLines(strings.Join(out, "\n"))
	return d
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
