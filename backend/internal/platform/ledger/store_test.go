package ledger

import (
	"context"
	"os"
	"testing"
	"time"

	"llm-proxy/internal/platform/db"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "ledger-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })
	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.DB().Close() })
	store, err := New(p)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStore_MigrateCreatesTables(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	row := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='icu_ledger'")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan tables: %v", err)
	}
	if count != 1 {
		t.Fatal("icu_ledger table not created")
	}
}

func TestStore_RecordAndGetTransaction(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	txn := ICUTransaction{
		TransactionID:   "test-txn-001",
		WorkspaceID:     "ws-1",
		ModelName:       "test-model",
		ProviderType:    "openai",
		ICUDebit:        100,
		RequestTokens:   50,
		ResponseTokens:  40,
		ReasoningTokens: 10,
	}
	if err := store.RecordTransaction(ctx, txn); err != nil {
		t.Fatalf("RecordTransaction: %v", err)
	}
}

func TestStore_RecordRefund_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	txn := ICUTransaction{
		TransactionID: "test-txn-refund",
		WorkspaceID:   "ws-1",
		ModelName:     "test-model",
		ProviderType:  "openai",
		ICUDebit:      100,
	}
	if err := store.RecordTransaction(ctx, txn); err != nil {
		t.Fatalf("RecordTransaction: %v", err)
	}
	if err := store.RecordRefund(ctx, "test-txn-refund", 100); err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}
	if err := store.RecordRefund(ctx, "test-txn-refund", 100); err == nil {
		t.Fatal("expected error on double refund")
	}
}

func TestStore_Balance_ZeroByDefault(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	balance, err := store.GetBalance(ctx, "ws-1", time.Now().UTC().Truncate(24*time.Hour))
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("expected balance 0, got %d", balance)
	}
}

func TestStore_UpdateAndGetBalance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	window := time.Now().UTC().Truncate(24 * time.Hour)
	if err := store.UpdateBalance(ctx, "ws-1", window, 500); err != nil {
		t.Fatalf("UpdateBalance: %v", err)
	}
	balance, err := store.GetBalance(ctx, "ws-1", window)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != 500 {
		t.Fatalf("expected balance 500, got %d", balance)
	}
}

func TestStore_UpdateBalanceAccumulates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	window := time.Now().UTC().Truncate(24 * time.Hour)
	store.UpdateBalance(ctx, "ws-1", window, 300)
	store.UpdateBalance(ctx, "ws-1", window, 200)
	balance, _ := store.GetBalance(ctx, "ws-1", window)
	if balance != 500 {
		t.Fatalf("expected accumulated balance 500, got %d", balance)
	}
}

func TestStore_SlotCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	slot := SlotRecord{
		ModelName: "test-model",
		SlotID:    0,
		Host:      "127.0.0.1",
		Port:      8081,
		CacheKey:  "abc123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if err := store.SaveSlot(ctx, slot); err != nil {
		t.Fatalf("SaveSlot: %v", err)
	}
	found, err := store.GetActiveSlot(ctx, "test-model", "127.0.0.1", 8081, "abc123")
	if err != nil {
		t.Fatalf("GetActiveSlot: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find active slot")
	}
	if found.SlotID != 0 {
		t.Fatalf("expected slot ID 0, got %d", found.SlotID)
	}
}

func TestStore_SlotExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	slot := SlotRecord{
		ModelName: "test-model",
		SlotID:    1,
		Host:      "127.0.0.1",
		Port:      8082,
		CacheKey:  "expired-key",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.SaveSlot(ctx, slot)
	found, err := store.GetActiveSlot(ctx, "test-model", "127.0.0.1", 8082, "expired-key")
	if err != nil {
		t.Fatalf("GetActiveSlot: %v", err)
	}
	if found != nil {
		t.Fatal("expected nil for expired slot (expires_at <= now)")
	}
}

func TestStore_ExpireSlots_DeletesExpired(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	slot := SlotRecord{
		ModelName: "test-model",
		SlotID:    2,
		Host:      "127.0.0.1",
		Port:      8083,
		CacheKey:  "old-key",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.SaveSlot(ctx, slot)
	if err := store.ExpireSlots(ctx); err != nil {
		t.Fatalf("ExpireSlots: %v", err)
	}
	found, _ := store.GetActiveSlot(ctx, "test-model", "127.0.0.1", 8083, "old-key")
	if found != nil {
		t.Fatal("expected slot deleted after ExpireSlots")
	}
}

func TestStore_EntityMetadata_CRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.SetEntityMetadata(ctx, "memory", "ws-1", "key1", []byte(`{"val":42}`)); err != nil {
		t.Fatalf("SetEntityMetadata: %v", err)
	}
	value, err := store.GetEntityMetadata(ctx, "memory", "ws-1", "key1")
	if err != nil {
		t.Fatalf("GetEntityMetadata: %v", err)
	}
	if string(value) != `{"val":42}` {
		t.Fatalf("expected '{\"val\":42}', got %s", string(value))
	}
	all, err := store.ListEntityMetadata(ctx, "memory", "ws-1")
	if err != nil {
		t.Fatalf("ListEntityMetadata: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if err := store.DeleteEntityMetadata(ctx, "memory", "ws-1", "key1"); err != nil {
		t.Fatalf("DeleteEntityMetadata: %v", err)
	}
	v, _ := store.GetEntityMetadata(ctx, "memory", "ws-1", "key1")
	if v != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestStore_EntityMetadata_Upsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.SetEntityMetadata(ctx, "knowledge", "ws-1", "key", []byte("v1"))
	store.SetEntityMetadata(ctx, "knowledge", "ws-1", "key", []byte("v2"))
	val, _ := store.GetEntityMetadata(ctx, "knowledge", "ws-1", "key")
	if string(val) != "v2" {
		t.Fatalf("expected 'v2' after upsert, got %s", string(val))
	}
}

func TestStore_PurgeTransactions_DeletesOnlyOld(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	oldTx := ICUTransaction{
		TransactionID: "old-txn", WorkspaceID: "ws-1", ModelName: "m", ProviderType: "p", ICUDebit: 10,
	}
	store.RecordTransaction(ctx, oldTx)
	_, _ = store.db.ExecContext(ctx, `UPDATE icu_ledger SET created_at = datetime('now', '-48 hours') WHERE transaction_id = 'old-txn'`)

	recentTx := ICUTransaction{
		TransactionID: "recent-txn", WorkspaceID: "ws-1", ModelName: "m", ProviderType: "p", ICUDebit: 10,
	}
	store.RecordTransaction(ctx, recentTx)

	cutoff := time.Now().Add(-24 * time.Hour)
	if err := store.PurgeTransactions(ctx, cutoff); err != nil {
		t.Fatalf("PurgeTransactions: %v", err)
	}

	var count int
	store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icu_ledger`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 remaining row, got %d", count)
	}

	var remainingID string
	store.db.QueryRowContext(ctx, `SELECT transaction_id FROM icu_ledger`).Scan(&remainingID)
	if remainingID != "recent-txn" {
		t.Fatalf("expected 'recent-txn' to remain, got %q", remainingID)
	}
}

func openTestDB(path string, t *testing.T) db.Provider {
	t.Helper()
	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return p
}

func TestStore_AutonomousCleaner(t *testing.T) {
	f, err := os.CreateTemp("", "ledger-cleaner-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	db1 := openTestDB(path, t)
	store1, err := New(db1)
	if err != nil {
		t.Fatalf("New store1: %v", err)
	}
	ctx := context.Background()

	expiredSlot := SlotRecord{
		ModelName: "test-model",
		SlotID:    1,
		Host:      "127.0.0.1",
		Port:      8081,
		CacheKey:  "expired-key",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if err := store1.SaveSlot(ctx, expiredSlot); err != nil {
		t.Fatalf("SaveSlot: %v", err)
	}

	txn := ICUTransaction{
		TransactionID: "old-txn",
		WorkspaceID:   "ws-1",
		ModelName:     "test-model",
		ProviderType:  "openai",
		ICUDebit:      100,
	}
	if err := store1.RecordTransaction(ctx, txn); err != nil {
		t.Fatalf("RecordTransaction: %v", err)
	}
	_, err = store1.db.ExecContext(ctx, `UPDATE icu_ledger SET created_at = datetime('now', '-48 hours') WHERE transaction_id = 'old-txn'`)
	if err != nil {
		t.Fatalf("backdate transaction: %v", err)
	}

	recentTxn := ICUTransaction{
		TransactionID: "recent-txn",
		WorkspaceID:   "ws-1",
		ModelName:     "test-model",
		ProviderType:  "openai",
		ICUDebit:      100,
	}
	if err := store1.RecordTransaction(ctx, recentTxn); err != nil {
		t.Fatalf("RecordTransaction recent: %v", err)
	}

	store1.Close()
	db1.DB().Close()

	db2 := openTestDB(path, t)
	store2, err := New(db2)
	if err != nil {
		t.Fatalf("New store2: %v", err)
	}
	defer store2.Close()
	defer db2.DB().Close()

	time.Sleep(50 * time.Millisecond)

	slot, err := store2.GetActiveSlot(ctx, "test-model", "127.0.0.1", 8081, "expired-key")
	if err != nil {
		t.Fatalf("GetActiveSlot: %v", err)
	}
	if slot != nil {
		t.Fatal("expected expired slot to be deleted by autonomous cleaner")
	}

	var oldExists int
	err = store2.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icu_ledger WHERE transaction_id = 'old-txn'`).Scan(&oldExists)
	if err != nil {
		t.Fatalf("check old transaction: %v", err)
	}
	if oldExists != 0 {
		t.Fatal("expected old transaction to be purged by autonomous cleaner")
	}

	var recentExists int
	err = store2.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icu_ledger WHERE transaction_id = 'recent-txn'`).Scan(&recentExists)
	if err != nil {
		t.Fatalf("check recent transaction: %v", err)
	}
	if recentExists != 1 {
		t.Fatal("expected recent transaction to remain")
	}
}


// TestPurgeBalances verifies old per-window balance rows are removed while
// recent ones are kept (the cleaner previously never purged icu_balances, so
// one row per workspace per window accumulated forever).
func TestPurgeBalances(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC().Add(-time.Hour)
	for _, ws := range []string{"ws-1", "ws-2"} {
		if err := store.UpdateBalance(ctx, ws, old, 10); err != nil {
			t.Fatalf("update old balance: %v", err)
		}
		if err := store.UpdateBalance(ctx, ws, recent, 20); err != nil {
			t.Fatalf("update recent balance: %v", err)
		}
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	if err := store.PurgeBalances(ctx, cutoff); err != nil {
		t.Fatalf("purge balances: %v", err)
	}

	// Old windows must be gone (GetBalance returns 0 for a missing row),
	// recent windows kept.
	for _, ws := range []string{"ws-1", "ws-2"} {
		if got, err := store.GetBalance(ctx, ws, old); err != nil || got != 0 {
			t.Errorf("expected old balance for %s to be purged, got %d err %v", ws, got, err)
		}
		if got, err := store.GetBalance(ctx, ws, recent); err != nil || got != 20 {
			t.Errorf("expected recent balance 20 for %s, got %d err %v", ws, got, err)
		}
	}
}
