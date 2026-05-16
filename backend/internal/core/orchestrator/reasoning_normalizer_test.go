package orchestrator

import "testing"

func TestNormalizer_Accumulate_ContentOnly(t *testing.T) {
	n := NewReasoningNormalizer()
	t1, r1 := n.Accumulate(StreamChunk{Content: "Hello world"})
	if t1 <= 0 {
		t.Fatalf("expected content tokens > 0, got %d", t1)
	}
	if r1 != 0 {
		t.Fatalf("expected 0 reasoning tokens, got %d", r1)
	}
}

func TestNormalizer_Accumulate_ReasoningOnly(t *testing.T) {
	n := NewReasoningNormalizer()
	t1, r1 := n.Accumulate(StreamChunk{ReasoningContent: "Let me think"})
	if r1 <= 0 {
		t.Fatalf("expected reasoning tokens > 0, got %d", r1)
	}
	if t1 != 0 {
		t.Fatalf("expected 0 content tokens for reasoning-only chunk, got %d", t1)
	}
}

func TestNormalizer_Accumulate_BothContentAndReasoning(t *testing.T) {
	n := NewReasoningNormalizer()
	t1, r1 := n.Accumulate(StreamChunk{Content: "Answer:", ReasoningContent: "Thinking"})
	if t1 <= 0 {
		t.Fatalf("expected content tokens > 0, got %d", t1)
	}
	if r1 <= 0 {
		t.Fatalf("expected reasoning tokens > 0, got %d", r1)
	}
}

func TestNormalizer_Anthropic_Sequence(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "", ReasoningContent: "Let me think step by step."})
	if !n.HasSeparateReasoning() {
		t.Fatal("expected reasoning detected")
	}

	ct, rt := n.Accumulate(StreamChunk{Content: "The answer is 42."})
	if ct <= 0 {
		t.Fatalf("expected content tokens for answer, got %d", ct)
	}
	if rt != 0 {
		t.Fatalf("expected 0 reasoning tokens for answer, got %d", rt)
	}
}

func TestNormalizer_OpenAI_UsageIgnored(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "Hello"})
	n.Accumulate(StreamChunk{Content: " world"})
	lastCT, lastRT := n.Accumulate(StreamChunk{})
	if lastCT != 0 || lastRT != 0 {
		t.Fatalf("usage chunk should be zero, got %d/%d", lastCT, lastRT)
	}
	tt, tr := n.Totals()
	if tt <= 0 {
		t.Fatalf("expected total text > 0, got %d", tt)
	}
	if tr != 0 {
		t.Fatalf("expected total reason = 0, got %d", tr)
	}
}

func TestNormalizer_DeepSeek_Sequence(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "", ReasoningContent: "Step 1: analyze."})
	n.Accumulate(StreamChunk{Content: "", ReasoningContent: "Step 2: conclude."})
	if !n.HasSeparateReasoning() {
		t.Fatal("expected reasoning detected")
	}
	ct, rt := n.Accumulate(StreamChunk{Content: "Final answer."})
	if ct <= 0 {
		t.Fatalf("expected content tokens for final answer, got %d", ct)
	}
	if rt != 0 {
		t.Fatalf("expected 0 reasoning tokens for final answer, got %d", rt)
	}
	tt, tr := n.Totals()
	if tt <= 0 {
		t.Fatal("expected total text > 0")
	}
	if tr <= 0 {
		t.Fatal("expected total reasoning > 0")
	}
}

func TestNormalizer_Totals(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "Hello ", ReasoningContent: "Let me think..."})
	n.Accumulate(StreamChunk{Content: "world"})
	tt, tr := n.Totals()
	if tt <= 0 {
		t.Fatalf("expected text > 0, got %d", tt)
	}
	if tr <= 0 {
		t.Fatalf("expected reason > 0, got %d", tr)
	}
}

func TestNormalizer_Reset(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "test", ReasoningContent: "think"})
	n.Reset()
	tt, tr := n.Totals()
	if tt != 0 || tr != 0 {
		t.Fatalf("expected 0/0 after reset, got %d/%d", tt, tr)
	}
	if n.seenReasoning {
		t.Fatal("expected reasoning flag reset")
	}
}

func TestNormalizer_UsageChunks_Idempotent(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "A"})
	ct1, rt1 := n.Accumulate(StreamChunk{})
	ct2, rt2 := n.Accumulate(StreamChunk{})
	if ct1 != 0 || rt1 != 0 {
		t.Fatalf("first usage chunk should be 0/0, got %d/%d", ct1, rt1)
	}
	if ct2 != 0 || rt2 != 0 {
		t.Fatalf("second usage chunk should be 0/0, got %d/%d", ct2, rt2)
	}
}

func TestNormalizer_HasSeparateReasoning_True(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{ReasoningContent: "think"})
	if !n.HasSeparateReasoning() {
		t.Fatal("expected HasSeparateReasoning true")
	}
}

func TestNormalizer_HasSeparateReasoning_False(t *testing.T) {
	n := NewReasoningNormalizer()
	n.Accumulate(StreamChunk{Content: "hello"})
	if n.HasSeparateReasoning() {
		t.Fatal("expected HasSeparateReasoning false")
	}
}
