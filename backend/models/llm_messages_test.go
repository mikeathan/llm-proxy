package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageExtractReasoning_Precedence(t *testing.T) {
	// ReasoningContent wins.
	m := Message{ReasoningContent: "rc", Reasoning: "r", ReasoningDetails: []ReasoningDetail{{Text: "d"}}}
	if got := m.ExtractReasoning(); got != "rc" {
		t.Errorf("expected ReasoningContent, got %q", got)
	}

	// Reasoning string next.
	m = Message{Reasoning: "r", ReasoningDetails: []ReasoningDetail{{Text: "d"}}}
	if got := m.ExtractReasoning(); got != "r" {
		t.Errorf("expected Reasoning, got %q", got)
	}

	// ReasoningDetails joined.
	m = Message{ReasoningDetails: []ReasoningDetail{{Text: "a"}, {Text: "b"}}}
	if got := m.ExtractReasoning(); got != "a\nb" {
		t.Errorf("expected joined details, got %q", got)
	}

	// None -> empty, and never fabricates.
	m = Message{Content: "just text"}
	if got := m.ExtractReasoning(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMessageExtractReasoning_InlineTags(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"before <think>hidden thought</think> after", "hidden thought"},
		{"<thinking>think hard</thinking>", "think hard"},
		{"<reasoning>reason</reasoning>", "reason"},
		{"<REASONING_SCRATCHPAD>notes</REASONING_SCRATCHPAD>", "notes"},
		{"no tags here", ""},
	}
	for _, c := range cases {
		m := Message{Content: c.content}
		if got := m.ExtractReasoning(); got != c.want {
			t.Errorf("ExtractReasoning(%q) = %q, want %q", c.content, got, c.want)
		}
	}
}

func TestMessageHasSeparateReasoning(t *testing.T) {
	if (Message{Content: "x"}).HasSeparateReasoning() {
		t.Error("content-only message should report no separate reasoning")
	}
	if !(Message{ReasoningContent: "x"}).HasSeparateReasoning() {
		t.Error("reasoning_content should report separate reasoning")
	}
}

// TestChatRequest_TemperatureOmittedWhenZero pins the "0 = unset" contract: a
// zero temperature is omitted from the wire so the provider default applies.
func TestChatRequest_TemperatureOmittedWhenZero(t *testing.T) {
	raw, err := json.Marshal(ChatRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "temperature") {
		t.Errorf("zero temperature must be omitted, got %s", raw)
	}
	raw, err = json.Marshal(ChatRequest{Model: "m", Temperature: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"temperature":0.1`) {
		t.Errorf("explicit temperature must be sent, got %s", raw)
	}
}
