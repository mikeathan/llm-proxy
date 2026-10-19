package tools

import (
	"strings"
	"testing"
	"unicode/utf8"

	"llm-proxy/internal/core/proxy"
)

func TestHTMLToText_KeepsArticleProseAndDropsMarkup(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><title>Sheffield - Wikipedia</title>
<meta charset="utf-8"><style>.x{color:red}</style><script>var a = "</p>";</script></head>
<body><nav><a href="/">Main page</a></nav>
<h1>Sheffield</h1><p>Sheffield is a city in <b>South Yorkshire</b>, England &amp; famous for steel.</p>
<script type="application/ld+json">{"headline":"city"}</script>
<p>It sits at the confluence of five rivers.</p><footer>Privacy policy</footer></body></html>`

	got := htmlToText(page)

	for _, want := range []string{"Sheffield - Wikipedia", "Sheffield is a city in South Yorkshire, England & famous for steel.", "It sits at the confluence of five rivers."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, banned := range []string{"<", ">", "color:red", "headline", "Main page", "Privacy policy", "var a"} {
		if strings.Contains(got, banned) {
			t.Errorf("%q should have been dropped from:\n%s", banned, got)
		}
	}
}

func TestHTMLToText_SeparatesBlocksAndCollapsesWhitespace(t *testing.T) {
	got := htmlToText("<p>one</p>\n\n\n   <p>two   words</p><ul><li>a</li><li>b</li></ul>")
	if want := "one\ntwo words\na\nb"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHTMLToText_HandlesAwkwardMarkup(t *testing.T) {
	cases := map[string]string{
		"comment":        "<p>a</p><!-- <p>hidden</p> --><p>b</p>",
		"gt in attr":     `<a title="x > y" href="/q">link text</a>`,
		"uppercase":      "<SCRIPT>bad()</SCRIPT><P>fine</P>",
		"unclosed skip":  "<p>kept</p><script>never closed",
		"stray lt":       "1 < 2 and 3 > 2",
		"truncated tag":  "<p>cut off here <a href=\"/x",
		"numeric entity": "caf&#233; &nbsp;au lait",
	}
	want := map[string]string{
		"comment":        "a\nb",
		"gt in attr":     "link text",
		"uppercase":      "fine",
		"unclosed skip":  "kept",
		"stray lt":       "1 < 2 and 3 > 2",
		"truncated tag":  "cut off here",
		"numeric entity": "café au lait",
	}
	for name, in := range cases {
		if got := htmlToText(in); got != want[name] {
			t.Errorf("%s: got %q, want %q", name, got, want[name])
		}
	}
}

func TestIsHTML(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{"declared html", "text/html; charset=utf-8", "anything", true},
		{"declared xhtml", "application/xhtml+xml", "anything", true},
		{"json is left alone", "application/json", `{"a":"<p>x</p>"}`, false},
		{"plain text is left alone", "text/plain", "<p>not markup</p>", false},
		{"no type, html body", "", "  <!DOCTYPE html><html><body>x</body></html>", true},
		{"no type, plain body", "", "just words", false},
	}
	for _, c := range cases {
		if got := isHTML(c.contentType, []byte(c.body)); got != c.want {
			t.Errorf("%s: isHTML = %v, want %v", c.name, got, c.want)
		}
	}
}

func wikiLikePage(city, fact string) string {
	chrome := strings.Repeat(`<link rel="stylesheet" href="/w/load.php?lang=en&amp;modules=site.styles"><script>RLCONF={"wgBreakFrames":false};</script>`, 12)
	return `<!DOCTYPE html><html><head><title>` + city + ` - Wikipedia</title>` + chrome + `</head><body><nav>` + chrome + `</nav>` +
		`<p>` + fact + `</p><footer>` + chrome + `</footer></body></html>`
}

func TestFetchedText_ArticleSurvivesTheResultCap(t *testing.T) {
	page := wikiLikePage("Leeds", "Leeds is a city in West Yorkshire.")

	capped := proxy.TruncateResultDefault

	if strings.Contains(capped(page), "Leeds is a city in West Yorkshire.") {
		t.Fatal("fixture should push the article out of the raw cap, or this test proves nothing")
	}
	if got := capped(fetchedText("text/html", []byte(page), false)); !strings.Contains(got, "Leeds is a city in West Yorkshire.") {
		t.Errorf("article lost to the cap: %q", got)
	}
}

func TestFetchedText_LeavesNonHTMLAlone(t *testing.T) {
	body := `{"a":"<p>x</p>"}`
	if got := fetchedText("application/json", []byte(body), false); got != body {
		t.Errorf("json changed: %q", got)
	}
	if got := fetchedText("text/plain", []byte("x"), true); !strings.HasSuffix(got, "(content truncated by network guardrails)") {
		t.Errorf("size-guardrail note missing: %q", got)
	}
}

func linkList(n int) string {
	var b strings.Builder
	b.WriteString("<ul>")
	for i := 0; i < n; i++ {
		b.WriteString(`<li><a href="/l">Lang` + strings.Repeat("x", i%3) + `</a></li>`)
	}
	return b.String() + "</ul>"
}

const proseBody = `<p>The forecast for Sheffield tomorrow is light rain with a high of 12 degrees and a low of 8 degrees, and a gentle breeze from the west all day long.</p>` +
	`<p>Further out, the week stays unsettled with showers on most days and temperatures staying close to the seasonal average for the region.</p>` +
	`<p>Rainfall totals are expected to stay modest, though the hills to the west will see more than the city centre itself on Thursday and Friday.</p>` +
	`<p>Overnight lows will dip towards the freezing mark in sheltered rural spots, so anyone travelling early on Saturday should expect some patches of ice on untreated roads.</p>`

func TestHTMLToText_DropsALongMenuButKeepsProse(t *testing.T) {
	got := htmlToText(linkList(30) + proseBody)
	if strings.Contains(got, "Lang") {
		t.Errorf("menu kept:\n%s", got)
	}
	if !strings.Contains(got, "light rain with a high of 12 degrees") {
		t.Errorf("prose lost:\n%s", got)
	}
}

func TestHTMLToText_KeepsAShortListAndALinkIndex(t *testing.T) {
	if got := htmlToText(linkList(5) + proseBody); !strings.Contains(got, "Lang") {
		t.Errorf("a short list of links is content, not a menu:\n%s", got)
	}
	if got := htmlToText(linkList(30)); strings.Count(got, "Lang") != 30 {
		t.Errorf("a page that is only links must keep them:\n%s", got)
	}
}

func TestHTMLToText_KeepsLinkedHeadlines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10; i++ {
		b.WriteString(`<p><a href="/n">A headline that is longer than any menu entry could be, number ` + string(rune('a'+i)) + `</a></p>`)
	}
	if got := htmlToText(b.String() + proseBody); strings.Count(got, "A headline") != 10 {
		t.Errorf("linked headlines are content:\n%s", got)
	}
}

func TestHTMLToText_DropsADropdownAndALinkCloud(t *testing.T) {
	cloud := strings.Repeat(`<a href="/t">Townname</a>`, 40)
	opts := "<select>" + strings.Repeat("<option>Optiontown</option>", 40) + "</select>"
	got := htmlToText("<p>" + cloud + "</p>" + opts + proseBody)
	if strings.Contains(got, "Townname") || strings.Contains(got, "Optiontown") {
		t.Errorf("dropdown or link cloud kept:\n%s", got)
	}
}

func TestHTMLToText_LeadsWithThePagesOwnSummary(t *testing.T) {
	page := `<html><head><title>Sheffield</title>
<meta name="description" content="City in South Yorkshire &amp; steel town">
<meta property="og:description" content="City in South Yorkshire &amp; steel town">
<meta content='A second description' name='description'>
<script type="application/ld+json">{"headline":"Sheffield, England","description":"a \"big\" city"}</script></head><body>` + proseBody + `</body></html>`
	lines := strings.Split(htmlToText(page), "\n")
	want := []string{"Sheffield", "Summary: City in South Yorkshire & steel town", "Summary: A second description", "Summary: Sheffield, England", `Summary: a "big" city`}
	for i, w := range want {
		if i >= len(lines) || lines[i] != w {
			t.Fatalf("line %d: got %q, want %q (all: %q)", i, lines[min(i, len(lines)-1)], w, lines)
		}
	}
}

func TestHTMLToText_ScopesToMainWhenItHoldsText(t *testing.T) {
	outside := `<div>` + strings.Repeat("Site banner words. ", 30) + `</div>`
	got := htmlToText(outside + "<main>" + proseBody + "</main>")
	if strings.Contains(got, "Site banner") || !strings.Contains(got, "light rain") {
		t.Errorf("not scoped to main:\n%s", got)
	}
	shell := htmlToText(outside + "<main><div id=app></div></main>")
	if !strings.Contains(shell, "Site banner") {
		t.Errorf("an empty <main> must fall back to the whole page:\n%s", shell)
	}
}

func TestPageHead_CutsAtTheEndOnARuneBoundary(t *testing.T) {
	if short := "short text"; pageHead(short) != short {
		t.Error("short text must be unchanged")
	}
	long := strings.Repeat("é", pageHeadChars)
	got := pageHead(long)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "the rest was cut]") || len(got) > pageHeadChars+60 {
		t.Errorf("bad cut: valid=%v len=%d", utf8.ValidString(got), len(got))
	}
}
