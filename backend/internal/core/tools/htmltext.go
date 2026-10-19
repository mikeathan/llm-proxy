package tools

import (
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"llm-proxy/internal/core/proxy"
)

// A fetched web page is mostly markup. The result cap is a few thousand characters, so cutting the raw page keeps
// the <head> and navigation and loses the article. htmlToText turns the page into its readable text first, so the
// cap, the model and the progress monitor all see prose.

// skippedElements are dropped with everything inside them: they hold code, styling or page chrome, not content.
var skippedElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true, "head": true, "template": true,
	"iframe": true, "nav": true, "footer": true, "aside": true, "select": true,
}

// blockElements start a new line of text.
var blockElements = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "ul": true, "ol": true, "tr": true, "table": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "section": true, "article": true,
	"blockquote": true, "pre": true, "hr": true, "dt": true, "dd": true, "header": true, "main": true,
}

// isHTML reports whether a response is an HTML page: by its declared type, or by sniffing the body when none is given.
func isHTML(contentType string, body []byte) bool {
	declared := strings.ToLower(contentType)
	switch {
	case strings.Contains(declared, "html"):
		return true
	case strings.TrimSpace(declared) != "":
		return false
	}
	return strings.Contains(http.DetectContentType(body), "text/html")
}

// Anchor text is wrapped in these private-use markers while the page is walked, so a line made only of links (a menu,
// a language list) can be told from prose. They never reach the output.
const (
	linkOpen  = '\ue000'
	linkClose = '\ue001'
)

const (
	menuRunLines   = 8   // this many link-only short lines in a row are a menu, not content
	menuLineChars  = 40  // a link-only line longer than this is a headline, not a menu entry
	minProseChars  = 500 // menus are dropped only when this much other text remains
	cloudMinChars  = 200 // a line this long that is almost all links, in many links, is a link cloud (a town list)
	cloudMinLinks  = 10
	minMainChars   = 200 // <main> is trusted as the page's content only when it holds this much text
	summaryLines   = 3   // at most this many summary lines lead a page
	summaryMaxChar = 300 // each shortened to this many characters
)

var markerStripper = strings.NewReplacer(string(linkOpen), "", string(linkClose), "")

// htmlToText returns the readable text of an HTML page: its title and its own summary (description meta tags,
// structured-data headline), then its main content one block per line. Scripts, styles, navigation and other page
// chrome are left out, long runs of link-only menu lines are dropped, and entities are decoded.
func htmlToText(src string) string {
	src = markerStripper.Replace(src)
	lines := mainLines(src)
	head := []string{}
	if title := pageTitle(src); title != "" {
		head = append(head, title)
	}
	head = append(head, pageSummary(src)...)
	return strings.Join(append(head, lines...), "\n")
}

// mainLines returns the page's content lines, from its <main> element when that holds real text, else from the whole
// page (a page without <main>, or a script-built shell with an empty one).
func mainLines(src string) []string {
	if open := indexFold(src, "<main", 0); open >= 0 {
		if end := indexFold(src, "</main", open); end >= 0 {
			if lines := contentLines(src[open:end]); len(strings.Join(lines, "")) >= minMainChars {
				return lines
			}
		}
	}
	return contentLines(src)
}

// contentLines walks markup into text lines, then drops menu runs when the page has prose besides.
func contentLines(src string) []string {
	var body strings.Builder
	for i := 0; i < len(src); {
		if src[i] != '<' || !startsMarkup(src, i) {
			body.WriteByte(src[i])
			i++
			continue
		}
		i = consumeMarkup(src, i, &body)
	}
	return dropMenus(readableLines(html.UnescapeString(body.String())))
}

const pageCutNote = "\n... [page text continues; the rest was cut]"

// pageHead keeps the start of a page's text. A web page leads with what it is about; its tail is references,
// categories and footers, so unlike other output it is cut at the end, not in the middle. The result cap is applied
// again to the JSON-encoded result (quotes, "&", "<" and newlines grow when encoded), so the cut is sized by the
// encoded length: the opening is then never re-cut.
func pageHead(text string) string {
	if encodedLen(text) <= proxy.MaxReturnChars {
		return text
	}
	cut := min(len(text), proxy.MaxReturnChars)
	for cut > 0 {
		for cut > 0 && cut < len(text) && !utf8.RuneStart(text[cut]) {
			cut--
		}
		over := encodedLen(text[:cut]+pageCutNote) - proxy.MaxReturnChars
		if over <= 0 {
			break
		}
		cut -= max(over/2, 1)
	}
	return text[:max(cut, 0)] + pageCutNote
}

// encodedLen is the length of s as a JSON string, which is how a tool result is capped.
func encodedLen(s string) int {
	encoded, err := json.Marshal(s)
	if err != nil {
		return len(s)
	}
	return len(encoded)
}

// startsMarkup tells a tag, comment or declaration from a stray "<" in text such as "1 < 2".
func startsMarkup(src string, i int) bool {
	if i+1 >= len(src) {
		return false
	}
	c := src[i+1]
	return c == '/' || c == '!' || c == '?' || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

// consumeMarkup handles the markup starting at src[i], writes the line break a block tag stands for, and returns
// the index after what it consumed. A skipped element is consumed whole, through its closing tag.
func consumeMarkup(src string, i int, out *strings.Builder) int {
	if strings.HasPrefix(src[i:], "<!--") {
		return pastMarker(src, "-->", i+4, false)
	}
	end := tagEnd(src, i)
	if end < 0 {
		return len(src)
	}
	name, closing, selfClosing := tagName(src[i+1 : end])
	switch {
	case !closing && !selfClosing && skippedElements[name]:
		return pastMarker(src, "</"+name, end+1, true)
	case name == "a":
		out.WriteRune(linkMarker(closing))
	case blockElements[name]:
		out.WriteByte('\n')
	}
	return end + 1
}

// pastMarker returns the index after the next occurrence of marker at or after from, and after the next ">" when
// toTagEnd is set (a closing tag); len(src) when there is none, which drops the rest of the page.
func pastMarker(src, marker string, from int, toTagEnd bool) int {
	at := indexFold(src, marker, from)
	if at < 0 {
		return len(src)
	}
	at += len(marker)
	if !toTagEnd {
		return at
	}
	if gt := strings.IndexByte(src[at:], '>'); gt >= 0 {
		return at + gt + 1
	}
	return len(src)
}

// tagEnd returns the index of the ">" that closes the tag opening at src[i], ignoring one inside a quoted attribute
// value; -1 when the page ends first.
func tagEnd(src string, i int) int {
	var quote byte
	for j := i + 1; j < len(src); j++ {
		c := src[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j
		}
	}
	return -1
}

// tagName reads the lower-cased element name from the inside of a tag (between "<" and ">").
func tagName(inside string) (name string, closing, selfClosing bool) {
	closing = strings.HasPrefix(inside, "/")
	selfClosing = strings.HasSuffix(inside, "/")
	inside = strings.TrimPrefix(inside, "/")
	if end := strings.IndexAny(inside, " \t\r\n/"); end >= 0 {
		inside = inside[:end]
	}
	return strings.ToLower(inside), closing, selfClosing
}

func linkMarker(closing bool) rune {
	if closing {
		return linkClose
	}
	return linkOpen
}

// readableLines splits text into lines and collapses runs of whitespace (non-breaking spaces included), dropping
// empty ones. Link markers stay in, for dropMenus.
func readableLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if words := strings.Fields(line); len(words) > 0 {
			lines = append(lines, strings.Join(words, " "))
		}
	}
	return lines
}

// linkOnlyLine reports whether everything on the line sits inside a link and the line is short: a menu entry.
func linkOnlyLine(line string) bool {
	plain := markerStripper.Replace(line)
	if utf8.RuneCountInString(plain) > menuLineChars {
		return false
	}
	return strings.HasPrefix(line, string(linkOpen)) && strings.HasSuffix(line, string(linkClose)) &&
		strings.Count(line, string(linkOpen)) == 1
}

// dropClouds removes the link-cloud lines, unless that would leave the page without prose (an index of links).
func dropClouds(lines []string) []string {
	kept := make([]string, 0, len(lines))
	prose := 0
	for _, line := range lines {
		if linkCloudLine(line) {
			continue
		}
		kept = append(kept, line)
		if !linkOnlyLine(line) {
			prose += len(markerStripper.Replace(line))
		}
	}
	if prose < minProseChars {
		return lines
	}
	return kept
}

// linkCloudLine reports a long line that is nearly all link text spread over many links: a list of places or tags
// run together, not prose.
func linkCloudLine(line string) bool {
	plain := markerStripper.Replace(line)
	if len(plain) < cloudMinChars || strings.Count(line, string(linkOpen)) < cloudMinLinks {
		return false
	}
	linked := 0
	for rest := line; ; {
		open := strings.IndexRune(rest, linkOpen)
		if open < 0 {
			break
		}
		rest = rest[open+len(string(linkOpen)):]
		end := strings.IndexRune(rest, linkClose)
		if end < 0 {
			break
		}
		linked += len(markerStripper.Replace(rest[:end]))
		rest = rest[end+len(string(linkClose)):]
	}
	return linked*100 >= len(plain)*95
}

// dropMenus removes runs of menuRunLines or more link-only lines (language lists, site menus) and the link markers.
// A page that is nothing but links (an index) keeps them: the drop applies only when prose remains.
func dropMenus(lines []string) []string {
	lines = dropClouds(lines)
	kept := make([]string, 0, len(lines))
	prose := 0
	for i := 0; i < len(lines); {
		run := 0
		for i+run < len(lines) && linkOnlyLine(lines[i+run]) {
			run++
		}
		if run >= menuRunLines {
			i += run
			continue
		}
		if run == 0 {
			run = 1
		}
		for _, line := range lines[i : i+run] {
			plain := markerStripper.Replace(line)
			if plain == "" {
				continue
			}
			if !linkOnlyLine(line) {
				prose += len(plain)
			}
			kept = append(kept, plain)
		}
		i += run
	}
	if prose < minProseChars {
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			if plain := markerStripper.Replace(line); plain != "" {
				out = append(out, plain)
			}
		}
		return out
	}
	return kept
}

var (
	metaTagRe     = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	metaKeyRe     = regexp.MustCompile(`(?is)\b(?:name|property)\s*=\s*["'](description|og:description)["']`)
	metaContentRe = regexp.MustCompile(`(?is)\bcontent\s*=\s*"([^"]*)"|\bcontent\s*=\s*'([^']*)'`)
	ldScriptRe    = regexp.MustCompile(`(?is)<script\b[^>]*application/ld\+json[^>]*>(.*?)</script`)
	ldFieldRe     = regexp.MustCompile(`"(?:headline|description)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

// pageSummary returns what the page says about itself: its description meta tags and the headline or description in
// its structured data (ld+json scripts only), at most summaryLines of them. Short, and often the one line that
// answers "what is this page".
func pageSummary(src string) []string {
	var found []string
	for _, tag := range metaTagRe.FindAllString(src, -1) {
		if metaKeyRe.MatchString(tag) {
			if m := metaContentRe.FindStringSubmatch(tag); m != nil {
				found = append(found, m[1]+m[2])
			}
		}
	}
	for _, script := range ldScriptRe.FindAllStringSubmatch(src, -1) {
		for _, m := range ldFieldRe.FindAllStringSubmatch(script[1], -1) {
			found = append(found, jsonStringValue(m[1]))
		}
	}
	return distinctSummaries(found)
}

// jsonStringValue decodes the inside of a JSON string literal (escapes such as \u0026 and \"); it returns the text as it
// is when it does not decode.
func jsonStringValue(inside string) string {
	var decoded string
	if err := json.Unmarshal([]byte(`"`+inside+`"`), &decoded); err != nil {
		return inside
	}
	return decoded
}

// distinctSummaries tidies, shortens and de-duplicates summary lines.
func distinctSummaries(found []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range found {
		line := strings.Join(strings.Fields(html.UnescapeString(f)), " ")
		if r := []rune(line); len(r) > summaryMaxChar {
			line = string(r[:summaryMaxChar])
		}
		if line != "" && !seen[line] {
			seen[line] = true
			out = append(out, "Summary: "+line)
		}
		if len(out) == summaryLines {
			break
		}
	}
	return out
}

// pageTitle returns the text of the page's <title>, if it has one.
func pageTitle(src string) string {
	open := indexFold(src, "<title", 0)
	if open < 0 {
		return ""
	}
	start := strings.IndexByte(src[open:], '>')
	if start < 0 {
		return ""
	}
	start += open + 1
	end := indexFold(src, "</title", start)
	if end < 0 {
		return ""
	}
	return strings.Join(strings.Fields(html.UnescapeString(src[start:end])), " ")
}

// indexFold is strings.Index ignoring ASCII case, from index from; -1 when absent.
func indexFold(s, sub string, from int) int {
	for j := from; j+len(sub) <= len(s); j++ {
		if strings.EqualFold(s[j:j+len(sub)], sub) {
			return j
		}
	}
	return -1
}
