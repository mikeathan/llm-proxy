package orchestrator

import (
	"context"
	"testing"
)

func TestStreamInterceptor_ProseRatio(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	contentStr := "Hello world, this is a test sentence with many chars."
	result := si.InterceptChunk(StreamChunk{
		Content: contentStr,
	})
	if result.TokensUsed <= 0 {
		t.Fatal("expected some tokens counted")
	}
	expectedMin := int(float64(len(contentStr)) * proseTokenRatio)
	if result.TokensUsed != expectedMin {
		t.Fatalf("expected ~%d prose tokens, got %d", expectedMin, result.TokensUsed)
	}
}

func TestStreamInterceptor_CodeSniff_TogglesInCodeBlock(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	if si.inCode {
		t.Fatal("expected not in code block initially")
	}

	chunk1 := "Some text ```go\nfunc main() {\n"
	si.InterceptChunk(StreamChunk{Content: chunk1})
	if !si.inCode {
		t.Fatal("expected inCode = true after opening backticks")
	}

	chunk2 := "fmt.Println(\"hello\")\n```\nmore text"
	si.InterceptChunk(StreamChunk{Content: chunk2})
	if si.inCode {
		t.Fatal("expected inCode = false after closing backticks")
	}
}

func TestStreamInterceptor_CodeRatio(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	si.inCode = true

	codeChunk := "x := 1; y := 2; z := x + y; fmt.Println(z);"
	result := si.InterceptChunk(StreamChunk{Content: codeChunk})
	expectedMin := int(float64(len(codeChunk)) * codeTokenRatio)
	if result.TokensUsed != expectedMin {
		t.Fatalf("code ratio: expected %d tokens, got %d", expectedMin, result.TokensUsed)
	}
}

func TestStreamInterceptor_ReasoningContent(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	result := si.InterceptChunk(StreamChunk{
		Content:          "",
		ReasoningContent: "Let me think about this problem step by step.",
	})
	if result.ReasoningUsed <= 0 {
		t.Fatal("expected reasoning tokens counted")
	}
	if result.TokensUsed != 0 {
		t.Fatalf("expected 0 content tokens when only reasoning, got %d", result.TokensUsed)
	}
}

func TestStreamInterceptor_BudgetTermination(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	result := si.InterceptChunkWithBudget(context.Background(),
		StreamChunk{}, 500, 100, 400, 200,
	)
	if !result.ShouldTerminate {
		t.Fatal("expected termination when tokUsed (500) > tokenBudget (400)")
	}
}

func TestStreamInterceptor_BudgetWithinLimit(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	result := si.InterceptChunkWithBudget(context.Background(),
		StreamChunk{}, 100, 50, 400, 200,
	)
	if result.ShouldTerminate {
		t.Fatal("expected no termination when under budget")
	}
}

func TestStreamInterceptor_ReasoningBudgetTermination(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	result := si.InterceptChunkWithBudget(context.Background(),
		StreamChunk{}, 50, 300, 400, 200,
	)
	if !result.ShouldTerminate {
		t.Fatal("expected termination when reasonUsed (300) > reasonBudget (200)")
	}
}

func TestStreamInterceptor_ZeroBudgets_NoTermination(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)
	result := si.InterceptChunkWithBudget(context.Background(),
		StreamChunk{}, 10000, 5000, 0, 0,
	)
	if result.ShouldTerminate {
		t.Fatal("expected no termination when budgets are zero (unlimited)")
	}
}

func TestStreamInterceptor_CodeSniff_ToggleEachFence(t *testing.T) {
	si := NewStreamInterceptor(nil, nil)

	si.InterceptChunk(StreamChunk{Content: "```"})
	if !si.inCode {
		t.Fatal("single fence should toggle to true")
	}

	si.InterceptChunk(StreamChunk{Content: "```"})
	if si.inCode {
		t.Fatal("second fence should toggle back to false")
	}

	si.InterceptChunk(StreamChunk{Content: "```"})
	if !si.inCode {
		t.Fatal("third fence should toggle to true again")
	}
}
