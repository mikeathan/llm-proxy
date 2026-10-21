package tools

import (
	"encoding/json"
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
	if got := capped(fetchedText("text/html", []byte(page), false, proxy.MaxReturnChars)); !strings.Contains(got, "Leeds is a city in West Yorkshire.") {
		t.Errorf("article lost to the cap: %q", got)
	}
}

func TestFetchedText_LeavesNonHTMLAlone(t *testing.T) {
	body := `{"a":"<p>x</p>"}`
	if got := fetchedText("application/json", []byte(body), false, proxy.MaxReturnChars); got != body {
		t.Errorf("json changed: %q", got)
	}
	if got := fetchedText("text/plain", []byte("x"), true, proxy.MaxReturnChars); !strings.HasSuffix(got, "(content truncated by network guardrails)") {
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
	want := []string{"Sheffield", "Summary: City in South Yorkshire & steel town", "Summary: A second description", "Summary: Sheffield, England"}
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
	if short := "short text"; pageHead(short, 0, proxy.MaxReturnChars) != short {
		t.Error("short text must be unchanged")
	}
	long := strings.Repeat("é", proxy.MaxReturnChars)
	got := pageHead(long, 0, proxy.MaxReturnChars)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "the rest was cut]") || encodedLen(got) > proxy.MaxReturnChars {
		t.Errorf("bad cut: valid=%v encoded=%d", utf8.ValidString(got), encodedLen(got))
	}
}

func TestHTMLToText_SummaryIsBoundedAndIgnoresOtherScripts(t *testing.T) {
	var state strings.Builder
	for i := 0; i < 20; i++ {
		state.WriteString(`{"description":"state blob entry ` + strings.Repeat("x", 40) + string(rune('a'+i)) + `"},`)
	}
	ld := `<script type="application/ld+json">{"headline":"Real headline","description":"Real description"}</script>`
	page := `<html><head><title>T</title><script>window.S=[` + state.String() + `]</script>` + ld + `</head><body>` + proseBody + `</body></html>`

	got := htmlToText(page)
	if strings.Contains(got, "state blob") {
		t.Errorf("non-ld+json script read as a summary:\n%s", got)
	}
	if n := strings.Count(got, "Summary:"); n != 2 {
		t.Errorf("got %d summary lines, want the 2 from the ld+json script:\n%s", n, got)
	}

	many := strings.Repeat(`<meta name="description" content="d`, 0)
	for i := 0; i < 20; i++ {
		many += `<meta name="description" content="distinct description number ` + string(rune('a'+i)) + `">`
	}
	got = pageHead(htmlToText("<html><head><title>T</title>"+many+"</head><body>"+proseBody+"</body></html>"), 0, proxy.MaxReturnChars)
	if n := strings.Count(got, "Summary:"); n > summaryLines {
		t.Errorf("%d summary lines, cap is %d", n, summaryLines)
	}
	if !strings.Contains(got, "light rain with a high of 12 degrees") {
		t.Errorf("summaries pushed the page content out of the head:\n%s", got)
	}
}

func TestFetchedText_FitsTheResultCapAfterJSONEscaping(t *testing.T) {
	var b strings.Builder
	b.WriteString("<main>")
	for i := 0; i < 400; i++ {
		b.WriteString(`<p>Tom &amp; "Jerry" <b>&lt;x&gt;</b> row ` + strings.Repeat("y", i%7) + `</p>`)
	}
	b.WriteString("</main>")
	encoded, err := json.Marshal(fetchedText("text/html", []byte(b.String()), false, proxy.MaxReturnChars))
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > proxy.MaxReturnChars {
		t.Errorf("encoded result is %d characters, over the %d cap that would cut it again", len(encoded), proxy.MaxReturnChars)
	}
}

func TestPageHead_ShortTextThatGrowsWhenEncodedDoesNotPanic(t *testing.T) {
	text := strings.Repeat(`"<&>"`, 580) // 2900 characters, about 5500 once JSON-encoded
	got := pageHead(text, 0, proxy.MaxReturnChars)
	if encodedLen(got) > proxy.MaxReturnChars || !strings.HasSuffix(got, "the rest was cut]") {
		t.Errorf("bad cut: encoded=%d", encodedLen(got))
	}
}

func TestHTMLToText_DecodesJSONEscapesInStructuredData(t *testing.T) {
	page := `<html><head><script type="application/ld+json">{"headline":"City in Tyne \u0026 Wear, \"England\""}</script></head><body>` + proseBody + `</body></html>`
	if got := htmlToText(page); !strings.Contains(got, `Summary: City in Tyne & Wear, "England"`) {
		t.Errorf("escapes not decoded:\n%s", got)
	}
}

func TestFetchedText_SizeLimitNoteKeepsTheResultUnderTheCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("<main>")
	for i := 0; i < 300; i++ {
		b.WriteString(`<p>Paragraph "number" <b>&amp;</b> text ` + strings.Repeat("w", i%9) + `</p>`)
	}
	b.WriteString("</main>")
	got := fetchedText("text/html", []byte(b.String()), true, proxy.MaxReturnChars)
	if !strings.HasSuffix(got, "(content truncated by network guardrails)") {
		t.Fatalf("the size-limit note is missing: %q", got[len(got)-80:])
	}
	if n := encodedLen(got); n > proxy.MaxReturnChars {
		t.Errorf("encodes to %d, over the %d cap, so the page opening would be cut again", n, proxy.MaxReturnChars)
	}
}

func TestFetchedText_PageHeadFollowsTheCallsCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("<main>")
	for i := 0; i < 400; i++ {
		b.WriteString(`<p>Paragraph "number" <b>&amp;</b> text ` + strings.Repeat("w", i%9) + `</p>`)
	}
	b.WriteString("</main>")
	page := []byte(b.String())

	small := fetchedText("text/html", page, false, proxy.MaxReturnChars)
	big := fetchedText("text/html", page, false, 8650)
	if len(big) <= len(small)+3000 {
		t.Errorf("a larger cap must carry more of the page: %d vs %d", len(big), len(small))
	}
	for _, c := range []struct {
		text string
		cap  int
	}{{small, proxy.MaxReturnChars}, {big, 8650}} {
		if n := encodedLen(c.text); n > c.cap {
			t.Errorf("encodes to %d, over its cap of %d", n, c.cap)
		}
	}
	withNote := fetchedText("text/html", page, true, 8650)
	if n := encodedLen(withNote); n > 8650 || !strings.HasSuffix(withNote, "(content truncated by network guardrails)") {
		t.Errorf("the size-limit note stays inside a larger cap: %d", n)
	}
}
