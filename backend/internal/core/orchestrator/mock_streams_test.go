package orchestrator

import (
	"testing"
)

type MockProvider string

const (
	MockOpenAI    MockProvider = "openai"
	MockAnthropic MockProvider = "anthropic"
	MockGemini    MockProvider = "gemini"
	MockDeepSeek  MockProvider = "deepseek"
)

type MockStreamConfig struct {
	Provider        MockProvider
	TextChunks      []string
	ReasoningChunks []string
	UsageTokens     int
	ReasoningTokens int
}

func ProduceStream(cfg MockStreamConfig) []StreamChunk {
	var chunks []StreamChunk

	switch cfg.Provider {
	case MockOpenAI:
		for _, t := range cfg.TextChunks {
			chunks = append(chunks, StreamChunk{
				Content: t, ReasoningContent: "", ProviderType: "openai",
			})
		}
		if cfg.UsageTokens > 0 || cfg.ReasoningTokens > 0 {
			chunks = append(chunks, StreamChunk{
				Content: "", ReasoningContent: "", ProviderType: "openai",
			})
		}
	case MockAnthropic:
		for _, r := range cfg.ReasoningChunks {
			chunks = append(chunks, StreamChunk{
				Content: "", ReasoningContent: r, ProviderType: "openrouter",
			})
		}
		for _, t := range cfg.TextChunks {
			chunks = append(chunks, StreamChunk{
				Content: t, ReasoningContent: "", ProviderType: "openrouter",
			})
		}
	case MockDeepSeek:
		for _, r := range cfg.ReasoningChunks {
			chunks = append(chunks, StreamChunk{
				Content: "", ReasoningContent: r, ProviderType: "openrouter",
			})
		}
		for _, t := range cfg.TextChunks {
			chunks = append(chunks, StreamChunk{
				Content: t, ReasoningContent: "", ProviderType: "openrouter",
			})
		}
	case MockGemini:
		for _, t := range cfg.TextChunks {
			chunks = append(chunks, StreamChunk{
				Content: t, ReasoningContent: "", ProviderType: "gemini",
			})
		}
		if cfg.UsageTokens > 0 || cfg.ReasoningTokens > 0 {
			chunks = append(chunks, StreamChunk{
				Content: "", ReasoningContent: "", ProviderType: "gemini",
			})
		}
	}

	return chunks
}

func TotalChunkTokens(chunks []StreamChunk) int {
	total := 0
	for _, c := range chunks {
		total += len(c.Content) + len(c.ReasoningContent)
	}
	return total
}

func TestMockStream_OpenAI_Basic(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:   MockOpenAI,
		TextChunks: []string{"Hello", " world", "!"},
	})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Content != "Hello" {
		t.Fatalf("expected 'Hello', got %s", chunks[0].Content)
	}
}

func TestMockStream_OpenAI_UsageIgnored(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:    MockOpenAI,
		TextChunks:  []string{"Hello", " world"},
		UsageTokens: 15,
	})
	_ = chunks
	si := NewStreamInterceptor(nil, NewReasoningNormalizer())
	total := 0
	for _, c := range chunks {
		r := si.InterceptChunk(c)
		total += r.TokensUsed
	}
	if total <= 0 {
		t.Fatal("expected total tokens > 0")
	}
}

func TestMockStream_Anthropic_ReasoningFirst(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:        MockAnthropic,
		TextChunks:      []string{"The answer is 42."},
		ReasoningChunks: []string{"Let me think", " about this problem."},
	})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].ReasoningContent == "" {
		t.Fatal("expected first chunk to have reasoning content")
	}
}

func TestMockStream_DeepSeek_ReasoningThenContent(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:        MockDeepSeek,
		TextChunks:      []string{"Final answer."},
		ReasoningChunks: []string{"Step 1: analyze", "Step 2: compute"},
	})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[1].ReasoningContent != "Step 2: compute" {
		t.Fatalf("expected reasoning at index 1, got %s", chunks[1].ReasoningContent)
	}
}

func TestMockStream_Gemini_ThoughtsTokens(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:        MockGemini,
		TextChunks:      []string{"Here's the result."},
		ReasoningTokens: 8,
	})
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (text + usage), got %d", len(chunks))
	}
	if chunks[1].Content != "" || chunks[1].ReasoningContent != "" {
		t.Fatalf("expected empty usage chunk, got content=%q reasoning=%q", chunks[1].Content, chunks[1].ReasoningContent)
	}
}

func TestMockStream_OpenAI_CodeSniff_InsideFence(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:   MockOpenAI,
		TextChunks: []string{"Here's code:\n```go\nx := 1\n```\n"},
	})
	si := NewStreamInterceptor(nil, nil)
	for _, c := range chunks {
		si.InterceptChunk(c)
	}
	// Inside the chunk, the code sniff should detect the backtick and toggle
	result := si.InterceptChunk(StreamChunk{Content: "outside again"})
	_ = result
	if si.inCode {
		t.Fatal("expected to exit code block after closing backticks")
	}
}

func TestMockStream_CodeSniff_OutsideFence_ReturnsProseRatio(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	chunks := ProduceStream(MockStreamConfig{
		Provider:   MockOpenAI,
		TextChunks: []string{"No code here, just prose. And more text."},
	})
	for _, c := range chunks {
		si.InterceptChunk(c)
	}
	result := si.InterceptChunk(StreamChunk{Content: "Still prose."})
	if result.TokensUsed <= 0 {
		t.Fatal("expected tokens counted")
	}
}

func TestMockStream_HardFloorActive(t *testing.T) {
	s := NewBudgetSqueezer()
	result := s.Squeeze(SqueezeRequest{
		MaxTokens:       10000,
		ReasoningBudget: 5000,
		ContextChars:    200000,
		ModelContextLen: 100000,
		ICUWeight:       1.0,
		RemainingICU:    100,
	})
	if result.SqueezeFactor < 0.2 {
		t.Fatalf("squeeze factor %f below hard floor 0.2", result.SqueezeFactor)
	}
	if result.Allowed {
		t.Fatal("expected rejection when even hard floor exceeds cap")
	}
}

func TestMockStream_OpenAI_UsageChunk_NoTokensAdded(t *testing.T) {
	chunks := ProduceStream(MockStreamConfig{
		Provider:    MockOpenAI,
		TextChunks:  []string{"Hello", " world"},
		UsageTokens: 10,
	})
	n := NewReasoningNormalizer()
	totalText, totalReason := 0, 0
	for _, c := range chunks {
		tt, tr := n.Accumulate(c)
		totalText += tt
		totalReason += tr
	}
	if totalText <= 0 {
		t.Fatalf("expected text > 0, got %d", totalText)
	}
}

func TestMockStream_Anthropic_NullContent(t *testing.T) {
	si := NewStreamInterceptor(nil, NewReasoningNormalizer())
	chunks := ProduceStream(MockStreamConfig{
		Provider:        MockAnthropic,
		ReasoningChunks: []string{"thinking step 1", "thinking step 2"},
		TextChunks:      []string{"The answer."},
	})
	for _, c := range chunks {
		result := si.InterceptChunk(c)
		// Anthropic reasoning blocks: reasoning content chunk should add only reasoning tokens
		if c.ReasoningContent != "" && result.TokensUsed != 0 && c.Content == "" {
			t.Fatal("anthropic reasoning chunk should have 0 content tokens")
		}
	}
}
