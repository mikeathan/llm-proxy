package ledger

import (
	"context"
	"testing"
	"time"
)

func TestCleaner_StartsAndStopsOnCancel(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	c := NewCleaner(store, 50*time.Millisecond, 24*time.Hour)
	go c.Start(ctx)

	// Let it tick a few times
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Should not panic or deadlock
}

func TestCleaner_NilSafeStore(t *testing.T) {
	// Should not panic with nil store (though NewCleaner requires non-nil
	// in practice — this is a boundary check on the Store methods).
	store := newTestStore(t)
	store.Close() // closed store should not panic, just log errors

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	c := NewCleaner(store, 20*time.Millisecond, 1*time.Hour)
	// Should not panic even though store is closed
	done := make(chan struct{})
	go func() {
		c.Start(ctx)
		close(done)
	}()
	<-done
}

func TestCleaner_CleansOldTransactions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	oldTx := ICUTransaction{
		TransactionID: "cleaner-old", WorkspaceID: "ws-1", ModelName: "m", ProviderType: "p", ICUDebit: 10,
	}
	store.RecordTransaction(ctx, oldTx)
	_, _ = store.db.ExecContext(ctx, `UPDATE icu_ledger SET created_at = datetime('now', '-48 hours') WHERE transaction_id = 'cleaner-old'`)

	recentTx := ICUTransaction{
		TransactionID: "cleaner-recent", WorkspaceID: "ws-1", ModelName: "m", ProviderType: "p", ICUDebit: 10,
	}
	store.RecordTransaction(ctx, recentTx)

	cleanCtx, cancel := context.WithCancel(context.Background())
	c := NewCleaner(store, 50*time.Millisecond, 24*time.Hour)
	go c.Start(cleanCtx)

	time.Sleep(150 * time.Millisecond)
	cancel()

	var count int
	store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icu_ledger`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 remaining row after cleaner run, got %d", count)
	}
}
