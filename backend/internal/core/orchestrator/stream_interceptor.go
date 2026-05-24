package orchestrator

// stream_interceptor counts tokens per SSE chunk with code-aware
// ratio switching (0.5 for prose, 1.0 inside ``` fences) and delegates
// to the reasoning normalizer for provider protocol detection.
// It signals early termination when per-turn token or reasoning budgets
// are exceeded, enabling reactive mid-stream budget enforcement.

import (
	"context"
	"strings"
)

const (
	proseTokenRatio = 0.5
	codeTokenRatio  = 1.0
	rollingWindowSize = 200
)

type StreamChunk struct {
	Content          string
	ReasoningContent string
	ProviderType     string
}

type StreamInterceptResult struct {
	ShouldTerminate bool
	TokensUsed      int
	ReasoningUsed   int
}

type StreamInterceptor struct {
	budget     *BudgetManager
	normalizer *ReasoningNormalizer
	inCode     bool
	window     strings.Builder
}

func NewStreamInterceptor(budget *BudgetManager, normalizer *ReasoningNormalizer) *StreamInterceptor {
	return &StreamInterceptor{
		budget:     budget,
		normalizer: normalizer,
	}
}

// InterceptChunk counts tokens in one SSE delta, applying the code-sniff
// ratio (1.0 inside ``` fences, 0.5 for prose).  Passes through the
// normalizer so reasoning tokens are separated from content tokens.
func (s *StreamInterceptor) InterceptChunk(chunk StreamChunk) StreamInterceptResult {
	s.updateCodeState(chunk.Content)

	ratio := proseTokenRatio
	if s.inCode {
		ratio = codeTokenRatio
	}

	var textTokens, reasoningTokens int
	if s.normalizer != nil {
		textTokens, reasoningTokens = s.normalizer.Accumulate(chunk)
		if textTokens > 0 && s.inCode {
			textTokens = int(float64(len(chunk.Content)) * ratio)
		}
	} else {
		textTokens = int(float64(len(chunk.Content)) * ratio)
		reasoningTokens = int(float64(len(chunk.ReasoningContent)) * proseTokenRatio)
	}

	return StreamInterceptResult{
		TokensUsed:    textTokens,
		ReasoningUsed: reasoningTokens,
	}
}

// InterceptChunkWithBudget checks running token totals against per-turn
// budgets and signals ShouldTerminate when either budget is exceeded.
func (s *StreamInterceptor) InterceptChunkWithBudget(ctx context.Context, chunk StreamChunk, tokUsed, reasonUsed, tokenBudget, reasonBudget int) StreamInterceptResult {
	_ = ctx
	result := StreamInterceptResult{
		TokensUsed:    tokUsed,
		ReasoningUsed: reasonUsed,
	}
	if tokenBudget > 0 && tokUsed > tokenBudget {
		result.ShouldTerminate = true
	}
	if reasonBudget > 0 && reasonUsed > reasonBudget {
		result.ShouldTerminate = true
	}
	return result
}

func (s *StreamInterceptor) updateCodeState(content string) {
	if content == "" {
		return
	}
	s.window.WriteString(content)
	win := s.window.String()
	if len(win) > rollingWindowSize {
		win = win[len(win)-rollingWindowSize:]
		s.window.Reset()
		s.window.WriteString(win)
	}
	if strings.Count(win, "```")%2 == 1 {
		s.inCode = !s.inCode
		s.window.Reset()
	}
}
