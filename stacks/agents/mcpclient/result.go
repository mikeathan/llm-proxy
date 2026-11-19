package mcpclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy-stacks/sdk"
)

// cutMarker ends a result that was cut at stacksdk.MaxDynamicResult.
const cutMarker = "\n[the answer was cut here]"

// Result turns what a tool answered into the text the model reads. The text blocks are kept in order; an image or audio block becomes one sentence
// that names its type and size (the model cannot use the bytes); an embedded resource's text is kept and a link becomes its name and address. A
// result with no text block but a structured one is that structure as JSON. isError is the tool's own failure, not a failure to reach it. Control
// characters are removed and the whole is cut at stacksdk.MaxDynamicResult.
func Result(r *mcp.CallToolResult) stacksdk.ToolResult {
	if r == nil {
		return stacksdk.ToolResult{IsError: true, Text: "the tool answered nothing"}
	}
	parts := make([]string, 0, len(r.Content))
	for _, c := range r.Content {
		if text := contentText(c); text != "" {
			parts = append(parts, text)
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" && r.StructuredContent != nil {
		if b, err := json.Marshal(r.StructuredContent); err == nil {
			text = string(b)
		}
	}
	if text == "" && !r.IsError {
		text = "the tool answered with no content"
	}
	text, cut := bound(clean(text), stacksdk.MaxDynamicResult)
	return stacksdk.ToolResult{Text: text, IsError: r.IsError, Truncated: cut}
}

func contentText(c mcp.Content) string {
	switch v := c.(type) {
	case mcp.TextContent:
		return v.Text
	case *mcp.TextContent:
		return v.Text
	case mcp.ImageContent:
		return binarySentence("image", v.MIMEType, v.Data)
	case *mcp.ImageContent:
		return binarySentence("image", v.MIMEType, v.Data)
	case mcp.AudioContent:
		return binarySentence("audio", v.MIMEType, v.Data)
	case *mcp.AudioContent:
		return binarySentence("audio", v.MIMEType, v.Data)
	case mcp.EmbeddedResource:
		return resourceText(v.Resource)
	case *mcp.EmbeddedResource:
		return resourceText(v.Resource)
	case mcp.ResourceLink:
		return linkSentence(v)
	case *mcp.ResourceLink:
		return linkSentence(*v)
	}
	return ""
}

func resourceText(r mcp.ResourceContents) string {
	switch v := r.(type) {
	case mcp.TextResourceContents:
		return v.Text
	case *mcp.TextResourceContents:
		return v.Text
	case mcp.BlobResourceContents:
		return binarySentence("resource", v.MIMEType, v.Blob)
	case *mcp.BlobResourceContents:
		return binarySentence("resource", v.MIMEType, v.Blob)
	}
	return ""
}

func linkSentence(l mcp.ResourceLink) string {
	return fmt.Sprintf("[resource: %s, %s]", oneLine(l.Name, 120), oneLine(l.URI, 300))
}

// binarySentence says what a block holds without showing it: its kind, its type and its size in bytes.
func binarySentence(kind, mime, b64 string) string {
	size := base64.StdEncoding.DecodedLen(len(b64))
	if mime == "" {
		mime = "unknown type"
	}
	return fmt.Sprintf("[%s: %s, %d bytes, not shown]", kind, oneLine(mime, 80), size)
}

// oneLine keeps a short, single-line piece of server text.
func oneLine(s string, limit int) string {
	return Text(strings.Join(strings.Fields(s), " "), limit)
}

// clean is stacksdk.CleanText without a cut: the same cleaning the core applies to the same text.
func clean(s string) string { return stacksdk.CleanText(s, len(s)) }

// bound cuts s at limit bytes on a character boundary and says whether it did. A result gets the marker; a short field does not.
func bound(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	if limit == stacksdk.MaxDynamicResult {
		return s[:end] + cutMarker, true
	}
	return s[:end], true
}

// Text cleans and cuts a piece of server text (a description, the instructions) for a screen or a prompt.
func Text(s string, limit int) string { return stacksdk.CleanText(s, limit) }
