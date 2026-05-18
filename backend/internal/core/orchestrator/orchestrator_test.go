package orchestrator

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewOrchestrator_CreatesDB(t *testing.T) {
	f, err := os.CreateTemp("", "orch-test-*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	orch, err := NewOrchestrator(path)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if orch.Budget == nil {
		t.Fatal("expected Budget not nil")
	}
	if orch.Interceptor == nil {
		t.Fatal("expected Interceptor not nil")
	}
	if orch.Store == nil {
		t.Fatal("expected Store not nil")
	}
	if orch.Normalizer == nil {
		t.Fatal("expected Normalizer not nil")
	}
	t.Cleanup(func() { orch.Store.Close() })
}

func TestOrchestrator_PreFlightThenStream_NilSafe(t *testing.T) {
	f, err := os.CreateTemp("", "orch-nil-*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	orch, err := NewOrchestrator(path)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	defer orch.Store.Close()

	ctx := context.Background()
	result, err := orch.Budget.PreFlightCheck(ctx, "ws-1", PreFlightRequest{
		ModelName:       "test-model",
		ProviderType:    "openai",
		ContextChars:    200,
		MaxTokens:       100,
		ReasoningBudget: 50,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed without cap")
	}
	if result.TransactionID != "" {
		t.Fatal("expected empty TransactionID without cap")
	}

	siResult := orch.Interceptor.InterceptChunk(StreamChunk{
		Content:          "Hello world",
		ReasoningContent: "",
		ProviderType:     "openai",
	})
	if siResult.TokensUsed <= 0 {
		t.Fatal("expected tokens counted")
	}
}

func TestOrchestrator_EndToEnd_WithCap(t *testing.T) {
	f, err := os.CreateTemp("", "orch-e2e-*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	orch, err := NewOrchestrator(path)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	defer orch.Store.Close()

	ctx := context.Background()
	orch.Budget.SetCap("ws-1", BudgetCap{
		MaxICU:     5000,
		WindowSize: 24 * time.Hour,
	})

	preflight, err := orch.Budget.PreFlightCheck(ctx, "ws-1", PreFlightRequest{
		ModelName:       "test-model",
		ProviderType:    "openai",
		ContextChars:    100,
		MaxTokens:       100,
		ReasoningBudget: 0,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if !preflight.Allowed {
		t.Fatal("expected allowed")
	}

	tokTotal := 0
	chunks := []string{"Hello", " world", "!"}
	for _, c := range chunks {
		siResult := orch.Interceptor.InterceptChunk(StreamChunk{Content: c})
		tokTotal += siResult.TokensUsed
	}
	if tokTotal <= 0 {
		t.Fatal("expected token accumulation")
	}

	termResult := orch.Interceptor.InterceptChunkWithBudget(ctx,
		StreamChunk{}, tokTotal, 0, 2, 0,
	)
	if !termResult.ShouldTerminate {
		t.Fatal("expected termination since tokTotal > tokenBudget(10)")
	}
}

func TestOrchestrator_CodeBlockFlow(t *testing.T) {
	f, err := os.CreateTemp("", "orch-code-*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	orch, err := NewOrchestrator(path)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	defer orch.Store.Close()

	proseContent := "Here is the code:"
	proseResult := orch.Interceptor.InterceptChunk(StreamChunk{
		Content: proseContent,
	})
	proseExpected := int(float64(len(proseContent)) * proseTokenRatio)
	if proseResult.TokensUsed != proseExpected {
		t.Fatalf("prose: expected %d, got %d", proseExpected, proseResult.TokensUsed)
	}

	orch.Interceptor.InterceptChunk(StreamChunk{Content: "```"})
	if !orch.Interceptor.inCode {
		t.Fatal("expected in code block")
	}

	codeContent := "x := 1 + 2"
	codeResult := orch.Interceptor.InterceptChunk(StreamChunk{
		Content: codeContent,
	})
	codeExpected := int(float64(len(codeContent)) * codeTokenRatio)
	if codeResult.TokensUsed != codeExpected {
		t.Fatalf("code: expected %d, got %d", codeExpected, codeResult.TokensUsed)
	}

	orch.Interceptor.InterceptChunk(StreamChunk{Content: "```"})
	if orch.Interceptor.inCode {
		t.Fatal("expected out of code block")
	}

	postContent := "That's the answer."
	postProseResult := orch.Interceptor.InterceptChunk(StreamChunk{
		Content: postContent,
	})
	postProseExpected := int(float64(len(postContent)) * proseTokenRatio)
	if postProseResult.TokensUsed != postProseExpected {
		t.Fatalf("post-prose: expected %d, got %d", postProseExpected, postProseResult.TokensUsed)
	}
}
