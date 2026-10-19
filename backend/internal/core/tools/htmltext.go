package tools

import (
	"html"
	"net/http"
	"strings"
)

// A fetched web page is mostly markup. The result cap is a few thousand characters, so cutting the raw page keeps
// the <head> and navigation and loses the article. htmlToText turns the page into its readable text first, so the
// cap, the model and the progress monitor all see prose.

// skippedElements are dropped with everything inside them: they hold code, styling or page chrome, not content.
var skippedElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true, "head": true, "template": true,
	"iframe": true, "nav": true, "footer": true, "aside": true,
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

// htmlToText returns the readable text of an HTML page: its title, then its body text one block per line,
// with scripts, styles, navigation and other page chrome left out and entities decoded.
func htmlToText(src string) string {
	var body strings.Builder
	for i := 0; i < len(src); {
		if src[i] != '<' || !startsMarkup(src, i) {
			body.WriteByte(src[i])
			i++
			continue
		}
		i = consumeMarkup(src, i, &body)
	}
	lines := readableLines(html.UnescapeString(body.String()))
	if title := pageTitle(src); title != "" {
		lines = append([]string{title}, lines...)
	}
	return strings.Join(lines, "\n")
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

// readableLines splits text into lines, collapses runs of whitespace (non-breaking spaces included) and drops empty ones.
func readableLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if words := strings.Fields(line); len(words) > 0 {
			lines = append(lines, strings.Join(words, " "))
		}
	}
	return lines
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
