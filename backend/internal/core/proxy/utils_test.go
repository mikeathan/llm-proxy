package proxy

import (
	"strings"
	"testing"
)

func TestDecodeToolArgs_EmptyReturnsNil(t *testing.T) {
	var target map[string]any
	if err := DecodeToolArgs("", &target); err != nil {
		t.Fatalf("empty input should return nil, got %v", err)
	}
	if target != nil {
		t.Fatalf("target should be untouched on empty input, got %v", target)
	}
}

func TestDecodeToolArgs_ValidPopulates(t *testing.T) {
	var target struct {
		Path string `json:"path"`
	}
	if err := DecodeToolArgs(`{"path":"a/b"}`, &target); err != nil {
		t.Fatalf("valid JSON should decode, got %v", err)
	}
	if target.Path != "a/b" {
		t.Fatalf("path = %q, want %q", target.Path, "a/b")
	}
}

func TestDecodeToolArgs_MalformedErrors(t *testing.T) {
	var target map[string]any
	if err := DecodeToolArgs(`{bad`, &target); err == nil {
		t.Fatal("malformed JSON should return an error")
	}
}

func TestTruncateResult_UnderLimitPassthrough(t *testing.T) {
	in := "short"
	if got := TruncateResult(in, 100, "\n...\n"); got != in {
		t.Fatalf("under-limit input should be returned unchanged, got %q", got)
	}
}

func TestTruncateResult_DefaultMarkerSubstitutesCount(t *testing.T) {
	in := strings.Repeat("x", 3000)
	out := TruncateResult(in, 2000, "")
	if !strings.Contains(out, "SYSTEM TRUNCATED 1000 CHARACTERS") {
		t.Fatalf("default marker count not substituted, got %q", out)
	}
	if !strings.HasPrefix(out, strings.Repeat("x", 1000)) {
		t.Fatalf("head not preserved at start of output")
	}
	if !strings.HasSuffix(out, strings.Repeat("x", 1000)) {
		t.Fatalf("tail not appended at end of output")
	}
}

func TestTruncateResult_NoVerbMarker(t *testing.T) {
	in := "abcdefghij"
	out := TruncateResult(in, 6, "\n...[Truncated]...\n")
	want := "abc" + "\n...[Truncated]...\n" + "hij"
	if out != want {
		t.Fatalf("no-verb marker output = %q, want %q", out, want)
	}
}

func TestTruncateResultDefault(t *testing.T) {
	in := strings.Repeat("y", 4000)
	out := TruncateResultDefault(in)
	if !strings.Contains(out, "SYSTEM TRUNCATED 1000 CHARACTERS") {
		t.Fatalf("TruncateResultDefault should use default marker, got %q", out)
	}
}
