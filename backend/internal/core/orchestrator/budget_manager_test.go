package orchestrator

import (
	"context"
	"os"
	"testing"
	"time"

	"llm-proxy/internal/platform/ledger"
)

func newTestBudgetManager(t *testing.T) (*BudgetManager, *ledger.Store) {
	t.Helper()
	f, err := os.CreateTemp("", "orchestrator-budget-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	store, err := ledger.Open(path)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	squeezer := NewBudgetSqueezer()
	bm := NewBudgetManager(store, squeezer)
	return bm, store
}

func TestBudgetManager_PreFlightCheck_NoCap_Allows(t *testing.T) {
	bm, _ := newTestBudgetManager(t)
	result, err := bm.PreFlightCheck(context.Background(), "ws-1", PreFlightRequest{
		ModelName:       "test",
		ProviderType:    "openai",
		ContextChars:    100,
		MaxTokens:       100,
		ReasoningBudget: 0,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when no cap set")
	}
	if result.TransactionID != "" {
		t.Fatal("expected empty transaction ID when no cap set")
	}
}

func TestBudgetManager_PreFlightCheck_UnderCap_Allows(t *testing.T) {
	bm, _ := newTestBudgetManager(t)
	bm.SetCap("ws-1", BudgetCap{
		MaxICU:     10000,
		WindowSize: 24 * time.Hour,
	})
	result, err := bm.PreFlightCheck(context.Background(), "ws-1", PreFlightRequest{
		ModelName:       "test",
		ProviderType:    "local",
		ContextChars:    100,
		MaxTokens:       50,
		ReasoningBudget: 0,
		ICUWeight:       1.0,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed when under cap")
	}
	if result.AllocatedICU <= 0 {
		t.Fatal("expected positive AllocatedICU")
	}
}

func TestBudgetManager_PreFlightCheck_OverCap_Rejects(t *testing.T) {
	bm, _ := newTestBudgetManager(t)
	bm.SetCap("ws-1", BudgetCap{
		MaxICU:     100,
		WindowSize: 24 * time.Hour,
	})
	result, err := bm.PreFlightCheck(context.Background(), "ws-1", PreFlightRequest{
		ModelName:       "test",
		ProviderType:    "openai",
		ContextChars:    100000,
		MaxTokens:       10000,
		ReasoningBudget: 5000,
		ICUWeight:       1.0,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected rejection when over cap")
	}
}

func TestBudgetManager_PreFlightCheck_TracksBalance(t *testing.T) {
	bm, store := newTestBudgetManager(t)
	bm.SetCap("ws-1", BudgetCap{
		MaxICU:     10000,
		WindowSize: 24 * time.Hour,
	})
	window := time.Now().UTC().Truncate(24 * time.Hour)
	ctx := context.Background()

	result, err := bm.PreFlightCheck(ctx, "ws-1", PreFlightRequest{
		ModelName:       "test",
		ProviderType:    "local",
		ContextChars:    100,
		MaxTokens:       50,
		ReasoningBudget: 0,
		ICUWeight:       1.0,
	})
	if err != nil {
		t.Fatalf("PreFlightCheck: %v", err)
	}
	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}

	balance, err := store.GetBalance(ctx, "ws-1", window)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance <= 0 {
		t.Fatal("balance should be non-zero after transaction")
	}
}

func TestBudgetManager_Refund(t *testing.T) {
	bm, store := newTestBudgetManager(t)
	ctx := context.Background()

	txn := ledger.ICUTransaction{
		TransactionID: "refund-test-001",
		WorkspaceID:   "ws-1",
		ModelName:     "test",
		ProviderType:  "openai",
		ICUDebit:      500,
	}
	if err := store.RecordTransaction(ctx, txn); err != nil {
		t.Fatalf("RecordTransaction: %v", err)
	}
	if err := bm.Refund(ctx, "refund-test-001"); err != nil {
		t.Fatalf("Refund: %v", err)
	}
}

func TestBudgetManager_Refund_NotFound(t *testing.T) {
	bm, _ := newTestBudgetManager(t)
	err := bm.Refund(context.Background(), "nonexistent-txn")
	if err == nil {
		t.Fatal("expected error on refund of nonexistent transaction")
	}
}

func TestBudgetManager_MultipleWorkspaces_Isolated(t *testing.T) {
	bm, store := newTestBudgetManager(t)
	ctx := context.Background()
	window := time.Now().UTC().Truncate(24 * time.Hour)

	bm.SetCap("ws-1", BudgetCap{MaxICU: 10000, WindowSize: 24 * time.Hour})
	bm.SetCap("ws-2", BudgetCap{MaxICU: 10000, WindowSize: 24 * time.Hour})

	bm.PreFlightCheck(ctx, "ws-1", PreFlightRequest{
		ModelName: "test", ProviderType: "local", ContextChars: 100, MaxTokens: 500, ReasoningBudget: 0,
	})

	balance2, _ := store.GetBalance(ctx, "ws-2", window)
	if balance2 != 0 {
		t.Fatalf("ws-2 balance should remain 0, got %d", balance2)
	}
}
