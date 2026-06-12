// Package orchestrator enforces resource budgets during LLM inference:
// pre-flight ICU checks, adaptive token squeezing, mid-stream termination,
// and provider-aware reasoning token counting.
//
// The Orchestrator is nil-safe — when absent the agent operates identically
// (zero-latency-impact regression gate).
package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/ledger"
)

// BudgetCap defines a spending limit for a workspace within a time window.
type BudgetCap struct {
	MaxICU     int64
	WindowSize time.Duration
}

// PreFlightRequest carries the projected resource usage before a call.
type PreFlightRequest struct {
	ModelName       string
	ProviderType    string
	ContextChars    int
	MaxTokens       int
	ReasoningBudget int
	ICUWeight       float64
}

// PreFlightResult carries the outcome of a budget check so the caller
// can adjust max_tokens / reasoning budget before sending the request.
type PreFlightResult struct {
	Allowed           bool
	AllocatedICU      int64
	RemainingICU      int64
	SqueezeFactor     float64
	TransactionID     string
	AdjustedMaxTokens int
	AdjustedReasoning int
	Reason            string
}

// BudgetManager enforces per-workspace ICU spending limits.
// It debits projected cost BEFORE the LLM call and refunds on failure,
// so transient infrastructure errors don't consume budget.
type BudgetManager struct {
	store    *ledger.Store
	squeezer *BudgetSqueezer
	mu       sync.RWMutex
	caps     map[string]BudgetCap
}

func NewBudgetManager(store *ledger.Store, squeezer *BudgetSqueezer) *BudgetManager {
	return &BudgetManager{
		store:    store,
		squeezer: squeezer,
		caps:     make(map[string]BudgetCap),
	}
}

func (b *BudgetManager) SetCap(workspaceID string, c BudgetCap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.caps[workspaceID] = c
}

func (b *BudgetManager) getCap(workspaceID string) (BudgetCap, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cap, ok := b.caps[workspaceID]
	return cap, ok
}

// PreFlightCheck debits the projected ICU cost from the workspace balance
// BEFORE the LLM request fires.  Caller MUST defer Refund() on error.
func (b *BudgetManager) PreFlightCheck(ctx context.Context, workspaceID string, req PreFlightRequest) (PreFlightResult, error) {
	c, hasCap := b.getCap(workspaceID)
	if !hasCap {
		return PreFlightResult{
			Allowed:           true,
			TransactionID:     "",
			SqueezeFactor:     1.0,
			AdjustedMaxTokens: req.MaxTokens,
			AdjustedReasoning: req.ReasoningBudget,
		}, nil
	}

	windowStart := time.Now().UTC().Truncate(c.WindowSize)
	balance, err := b.store.GetBalance(ctx, workspaceID, windowStart)
	if err != nil {
		return PreFlightResult{}, fmt.Errorf("get balance: %w", err)
	}

	if balance >= c.MaxICU {
		return PreFlightResult{Allowed: false, Reason: "ICU cap exceeded for window"}, nil
	}

	remaining := c.MaxICU - balance
	result := b.squeezer.Squeeze(SqueezeRequest{
		MaxTokens:       req.MaxTokens,
		ReasoningBudget: req.ReasoningBudget,
		ContextChars:    req.ContextChars,
		ICUWeight:       req.ICUWeight,
		RemainingICU:    remaining,
	})

	preflight := PreFlightResult{
		Allowed:           result.Allowed,
		SqueezeFactor:     result.SqueezeFactor,
		AdjustedMaxTokens: result.AdjustedMaxTokens,
		AdjustedReasoning: result.AdjustedReasoning,
		AllocatedICU:      result.AllocatedICU,
		TransactionID:     fmt.Sprintf("txn_%d", time.Now().UnixNano()),
		Reason:            result.Reason,
	}

	if !preflight.Allowed {
		return preflight, nil
	}

	allocatedICU := preflight.AllocatedICU
	if allocatedICU <= 0 {
		allocatedICU = int64(float64(req.MaxTokens+req.ReasoningBudget+req.ContextChars/2) * req.ICUWeight)
		preflight.AllocatedICU = allocatedICU
	}

	if err := b.store.RecordTransaction(ctx, ledger.ICUTransaction{
		TransactionID:   preflight.TransactionID,
		WorkspaceID:     workspaceID,
		ModelName:       req.ModelName,
		ProviderType:    req.ProviderType,
		ICUDebit:        allocatedICU,
		RequestTokens:   req.ContextChars / 2,
		ResponseTokens:  req.MaxTokens,
		ReasoningTokens: req.ReasoningBudget,
	}); err != nil {
		return PreFlightResult{}, fmt.Errorf("record transaction: %w", err)
	}

	if err := b.store.UpdateBalance(ctx, workspaceID, windowStart, allocatedICU); err != nil {
		return PreFlightResult{}, fmt.Errorf("update balance: %w", err)
	}

	return preflight, nil
}

// Refund credits ICU back for a previously allocated transaction.
// Idempotent: calling twice on the same txn is a no-op.
func (b *BudgetManager) Refund(ctx context.Context, transactionID string) error {
	return b.store.RecordRefund(ctx, transactionID, 0)
}

// Orchestrator wires the budget manager, stream interceptor, slot manager
// and ledger store into a single nil-safe component.  When nil, the agent
// operates identically (zero-latency-impact regression gate).
type Orchestrator struct {
	Budget      *BudgetManager
	Store       *ledger.Store
	Interceptor *StreamInterceptor
	Normalizer  *ReasoningNormalizer
	Slots       *SlotManager
}

func New(store *ledger.Store) *Orchestrator {
	squeezer := NewBudgetSqueezer()
	budget := NewBudgetManager(store, squeezer)
	normalizer := NewReasoningNormalizer()
	interceptor := NewStreamInterceptor(budget, normalizer)
	slots := NewSlotManager(store)

	return &Orchestrator{
		Budget:      budget,
		Store:       store,
		Interceptor: interceptor,
		Normalizer:  normalizer,
		Slots:       slots,
	}
}

func NewOrchestrator(p db.Provider) (*Orchestrator, error) {
	store, err := ledger.New(p)
	if err != nil {
		return nil, err
	}
	return New(store), nil
}
