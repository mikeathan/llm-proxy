package orchestrator

// reasoning_normalizer separates reasoning tokens from content tokens by
// detecting the stream mode (separate reasoning deltas or inline content)
// from the data itself.  No provider-name-to-format mapping needed, so
// OpenRouter and NVIDIA routing work transparently.

// ReasoningNormalizer separates reasoning tokens from content tokens in
// streaming LLM responses.  It detects the reasoning mode from the stream
// data itself, not from a provider name — this lets it handle any model
// routed through OpenRouter, NVIDIA, or any other gateway without
// maintaining a provider→format mapping.
//
// Modes detected at stream time:
//   Separate reasoning — reasoning_content deltas arrive before content
//     (Anthropic Claude, DeepSeek R1, any model with thinking blocks)
//   Standard           — no reasoning_content deltas (GPT-4o, Gemini, etc)
//   Usage chunk        — empty delta carrying token counts (both modes)
type ReasoningNormalizer struct {
	seenReasoning bool
	usageSeen     bool
	totalReason   int
	totalText     int
}

func NewReasoningNormalizer() *ReasoningNormalizer {
	return &ReasoningNormalizer{}
}

// Accumulate counts tokens for one stream chunk.  Content and reasoning
// tokens are counted separately: reasoning chunk content is never
// double-counted as content tokens.  Empty usage chunks are zeroed.
func (n *ReasoningNormalizer) Accumulate(chunk StreamChunk) (contentTokens, reasonTokens int) {
	proseRatio := 0.5

	hasContent := chunk.Content != ""
	hasReasoning := chunk.ReasoningContent != ""

	if hasReasoning {
		n.seenReasoning = true
		reasonTokens = int(float64(len(chunk.ReasoningContent)) * proseRatio)
	}

	if hasContent && !hasReasoning {
		contentTokens = int(float64(len(chunk.Content)) * proseRatio)
	}

	if hasContent && hasReasoning {
		contentTokens = int(float64(len(chunk.Content)) * proseRatio)
		reasonTokens = int(float64(len(chunk.ReasoningContent)) * proseRatio)
	}

	if !hasContent && !hasReasoning {
		if !n.usageSeen {
			n.usageSeen = true
		}
	}

	n.totalText += contentTokens
	n.totalReason += reasonTokens
	return
}

func (n *ReasoningNormalizer) Totals() (int, int) {
	return n.totalText, n.totalReason
}

func (n *ReasoningNormalizer) Reset() {
	n.totalText = 0
	n.totalReason = 0
	n.seenReasoning = false
	n.usageSeen = false
}

func (n *ReasoningNormalizer) HasSeparateReasoning() bool {
	return n.seenReasoning
}
