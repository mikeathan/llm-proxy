package mcpclient

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy-stacks/sdk"
)

func TestResult_TextBlocksKeepTheirOrder(t *testing.T) {
	r := Result(&mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("one"), mcp.NewTextContent("two")}})
	if r.Text != "one\ntwo" || r.IsError || r.Truncated {
		t.Fatalf("got %+v", r)
	}
}

func TestResult_AToolErrorIsTheToolsOwnWords(t *testing.T) {
	r := Result(mcp.NewToolResultError("no such file"))
	if !r.IsError || r.Text != "no such file" {
		t.Fatalf("got %+v", r)
	}
}

func TestResult_ImageAndAudioBecomeASentenceWithTheirSize(t *testing.T) {
	data := base64.StdEncoding.EncodeToString(make([]byte, 300))
	r := Result(&mcp.CallToolResult{Content: []mcp.Content{
		mcp.NewImageContent(data, "image/png"), mcp.AudioContent{Type: "audio", Data: data, MIMEType: "audio/wav"},
	}})
	for _, want := range []string{"[image: image/png, 300 bytes, not shown]", "[audio: audio/wav, 300 bytes, not shown]"} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("%q missing from %q", want, r.Text)
		}
	}
	if strings.Contains(r.Text, data) {
		t.Error("the bytes must never reach the model")
	}
}

func TestResult_EmbeddedResourceAndLink(t *testing.T) {
	r := Result(&mcp.CallToolResult{Content: []mcp.Content{
		mcp.NewEmbeddedResource(mcp.TextResourceContents{URI: "file:///a", Text: "inside"}),
		mcp.NewResourceLink("file:///b", "b file", "", ""),
	}})
	if !strings.Contains(r.Text, "inside") || !strings.Contains(r.Text, "[resource: b file, file:///b]") {
		t.Fatalf("got %q", r.Text)
	}
}

func TestResult_StructuredContentWhenThereIsNoText(t *testing.T) {
	r := Result(&mcp.CallToolResult{StructuredContent: map[string]any{"a": 1}})
	if r.Text != `{"a":1}` {
		t.Fatalf("got %q", r.Text)
	}
	withText := Result(&mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("words")}, StructuredContent: map[string]any{"a": 1}})
	if withText.Text != "words" {
		t.Errorf("text is preferred when it exists, got %q", withText.Text)
	}
}

func TestResult_NothingAnsweredIsSaidInWords(t *testing.T) {
	if r := Result(nil); !r.IsError || r.Text == "" {
		t.Errorf("nil: %+v", r)
	}
	if r := Result(&mcp.CallToolResult{}); r.IsError || r.Text == "" {
		t.Errorf("empty: %+v", r)
	}
}

func TestResult_ControlCharactersAreRemovedAndTheAnswerIsCut(t *testing.T) {
	r := Result(&mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("a\x1b[31mb\x00c\td\n")}})
	if strings.ContainsAny(r.Text, "\x1b\x00") || !strings.Contains(r.Text, "\t") {
		t.Errorf("control characters: %q", r.Text)
	}
	big := strings.Repeat("é", stacksdk.MaxDynamicResult)
	cut := Result(&mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent(big)}})
	if !cut.Truncated || len(cut.Text) > stacksdk.MaxDynamicResult+len(cutMarker) || !strings.HasSuffix(cut.Text, cutMarker) {
		t.Errorf("a big answer must be cut with the marker: truncated=%v len=%d", cut.Truncated, len(cut.Text))
	}
	if strings.ContainsRune(strings.TrimSuffix(cut.Text, cutMarker), utf8.RuneError) {
		t.Error("the cut must fall on a character boundary")
	}
}

func TestText_CutsOnACharacterBoundaryWithoutAMarker(t *testing.T) {
	if got := Text("héllo", 2); got != "h" {
		t.Errorf("got %q", got)
	}
	if got := Text("abc", 10); got != "abc" {
		t.Errorf("got %q", got)
	}
}
