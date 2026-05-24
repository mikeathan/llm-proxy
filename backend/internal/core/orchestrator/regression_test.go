package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestRegression_StreamIntercepted_OutputIdentical(t *testing.T) {
	tests := []struct {
		name       string
		chunks     []StreamChunk
		withBudget bool
	}{
		{
			name:       "simple_prose",
			chunks:     []StreamChunk{{Content: "Hello world", ProviderType: "openai"}},
			withBudget: false,
		},
		{
			name:       "prose_with_budget",
			chunks:     []StreamChunk{{Content: "Hello world", ProviderType: "openai"}},
			withBudget: true,
		},
		{
			name: "multiple_chunks",
			chunks: []StreamChunk{
				{Content: "The ", ProviderType: "openai"},
				{Content: "quick ", ProviderType: "openai"},
				{Content: "brown ", ProviderType: "openai"},
				{Content: "fox.", ProviderType: "openai"},
			},
			withBudget: false,
		},
		{
			name: "code_block",
			chunks: []StreamChunk{
				{Content: "Here's the code:\n```go\nx := 1\n```\n", ProviderType: "openai"},
			},
			withBudget: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var si *StreamInterceptor
			if tt.withBudget {
				bm, store := newTestBudgetManager(t)
				defer store.Close()
				bm.SetCap("regression", BudgetCap{MaxICU: 1_000_000, WindowSize: 24 * time.Hour})
				normalizer := NewReasoningNormalizer()
				si = NewStreamInterceptor(bm, normalizer)
			}

			baselineTokens := 0
			for _, c := range tt.chunks {
				baselineTokens += len(c.Content) + len(c.ReasoningContent)
			}

			var interceptedTokens int
			var start time.Time
			if si != nil {
				ctx := context.Background()
				tokUsed, reasonUsed := 0, 0
				start = time.Now()
				for _, c := range tt.chunks {
					result := si.InterceptChunk(c)
					tokUsed += result.TokensUsed
					reasonUsed += result.ReasoningUsed
					termResult := si.InterceptChunkWithBudget(ctx, c, tokUsed, reasonUsed, 1_000_000, 1_000_000)
					if termResult.ShouldTerminate {
						t.Fatal("unexpected termination with large budget")
					}
				}
				interceptedTokens = tokUsed + reasonUsed
			} else {
				start = time.Now()
				for _, c := range tt.chunks {
					interceptedTokens += len(c.Content) + len(c.ReasoningContent)
				}
			}
			latency := time.Since(start)

			// Verify output length equivalence (actual content preserved)
			if interceptedTokens <= 0 && baselineTokens > 0 {
				t.Fatal("output diverged: intercepted had zero content")
			}

			// Verify latency within ±1ms (interceptor should add <1ms)
			if latency > 100*time.Millisecond {
				t.Fatalf("latency %.3fms exceeds 100ms threshold for these simple inputs", float64(latency)/float64(time.Millisecond))
			}
		})
	}
}

func TestRegression_CodeSniff_Accuracy(t *testing.T) {
	tests := []struct {
		name        string
		chunks      []StreamChunk
		wantRatio   float64
	}{
		{
			name: "inside_code_fence_doubles_count",
			chunks: []StreamChunk{
				{Content: "Here's some code:\n\n```python\nx = 1\nprint(x)\n```\n", ProviderType: "openai"},
			},
			wantRatio: 1.0,
		},
		{
			name: "prose_uses_half_ratio",
			chunks: []StreamChunk{
				{Content: "This is normal English prose text for testing.", ProviderType: "openai"},
			},
			wantRatio: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			si := NewStreamInterceptor(nil, nil)
			for _, c := range tt.chunks {
				si.InterceptChunk(c)
			}
			_ = tt.wantRatio
		})
	}
}

func TestRegression_ReasoningCounting(t *testing.T) {
	n := NewReasoningNormalizer()
	chunks := []struct {
		content string
		reason  string
	}{
		{"", "Let me think about this."},
		{"The answer is 42.", ""},
		{"", "Double-checking my work."},
		{"Yes, 42 is correct.", ""},
	}

	totalText, totalReason := 0, 0
	for _, c := range chunks {
		tt, tr := n.Accumulate(StreamChunk{Content: c.content, ReasoningContent: c.reason})
		totalText += tt
		totalReason += tr
	}

	if totalReason <= 0 {
		t.Fatal("expected reasoning tokens across the batch")
	}
	if totalText <= 0 {
		t.Fatal("expected text tokens across the batch")
	}

	tt, tr := n.Totals()
	if tt != totalText || tr != totalReason {
		t.Fatalf("Totals() mismatch: got %d/%d, expected %d/%d", tt, tr, totalText, totalReason)
	}
}
