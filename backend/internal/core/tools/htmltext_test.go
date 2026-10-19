package tools

import (
	"strings"
	"testing"

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
