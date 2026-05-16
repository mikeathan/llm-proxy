// Package ledger provides an operational SQLite store for tracking ICU
// transactions, model slot state, and generic entity metadata.  It is
// designed as a decoupled persistence layer — future memory and knowledge
// modules consume the same entity_metadata table without schema changes.
//
// WAL mode with a 5s busy timeout handles concurrent agentic loops.
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const migrateSQL = `
CREATE TABLE IF NOT EXISTS active_slots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    model_name      TEXT NOT NULL,
    slot_id         INTEGER NOT NULL,
    host            TEXT NOT NULL,
    port            INTEGER NOT NULL,
    cache_key       TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    last_used_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at      DATETIME NOT NULL,
    UNIQUE(model_name, host, port)
);
CREATE INDEX IF NOT EXISTS idx_slots_model ON active_slots(model_name);
CREATE INDEX IF NOT EXISTS idx_slots_expiry ON active_slots(expires_at);

CREATE TABLE IF NOT EXISTS icu_ledger (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id   TEXT NOT NULL UNIQUE,
    workspace_id     TEXT NOT NULL,
    model_name       TEXT NOT NULL,
    provider_type    TEXT NOT NULL,
    icu_debit        INTEGER NOT NULL,
    icu_credit       INTEGER NOT NULL DEFAULT 0,
    refund_status    TEXT NOT NULL DEFAULT 'none',
    request_tokens   INTEGER NOT NULL DEFAULT 0,
    response_tokens  INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_ledger_workspace ON icu_ledger(workspace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_window ON icu_ledger(workspace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_txn ON icu_ledger(transaction_id);

CREATE TABLE IF NOT EXISTS icu_balances (
    workspace_id TEXT NOT NULL,
    window_start DATETIME NOT NULL,
    total_icu    INTEGER NOT NULL DEFAULT 0,
    last_updated DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (workspace_id, window_start)
);

CREATE TABLE IF NOT EXISTS entity_metadata (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       BLOB NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(entity_type, entity_id, key)
);
CREATE INDEX IF NOT EXISTS idx_meta_lookup ON entity_metadata(entity_type, entity_id);
`

// Store is an operational SQLite database for runtime state — ICU ledger,
// slot tracking, and entity metadata.  Not for config (that lives in
// storage.Store[T]).
type Store struct {
	db *sql.DB
}

// ICUTransaction records a single debit against a workspace budget.
type ICUTransaction struct {
	TransactionID   string
	WorkspaceID     string
	ModelName       string
	ProviderType    string
	ICUDebit        int64
	ICUPerTokenBase float64
	RequestTokens   int
	ResponseTokens  int
	ReasoningTokens int
}

// SlotRecord tracks a persisted llama.cpp KV-cache slot so the
// system prompt doesn't need re-processing on every request.
type SlotRecord struct {
	ModelName  string
	SlotID     int
	Host       string
	Port       int
	CacheKey   string
	ExpiresAt  time.Time
	LastUsedAt time.Time
}

func Open(path string) (*Store, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("ledger open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(migrateSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("ledger migrate: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) RecordTransaction(ctx context.Context, tx ICUTransaction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO icu_ledger (transaction_id, workspace_id, model_name, provider_type, icu_debit, request_tokens, response_tokens, reasoning_tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tx.TransactionID, tx.WorkspaceID, tx.ModelName, tx.ProviderType, tx.ICUDebit, tx.RequestTokens, tx.ResponseTokens, tx.ReasoningTokens,
	)
	if err != nil {
		return fmt.Errorf("record transaction: %w", err)
	}
	return nil
}

// RecordRefund marks a previously debited transaction as fully refunded.
// Idempotency is enforced at the SQL level — refund_status = 'none' check.
func (s *Store) RecordRefund(ctx context.Context, transactionID string, amount int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE icu_ledger SET icu_credit = icu_credit + ?, refund_status = 'full' WHERE transaction_id = ? AND refund_status = 'none'`,
		amount, transactionID,
	)
	if err != nil {
		return fmt.Errorf("record refund: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("refund: transaction %s not found or already refunded", transactionID)
	}
	return nil
}

func (s *Store) GetBalance(ctx context.Context, workspaceID string, windowStart time.Time) (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT total_icu FROM icu_balances WHERE workspace_id = ? AND window_start = ?`,
		workspaceID, windowStart.UTC().Format("2006-01-02 15:04:05"),
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	if total.Valid {
		return total.Int64, nil
	}
	return 0, nil
}

func (s *Store) UpdateBalance(ctx context.Context, workspaceID string, windowStart time.Time, delta int64) error {
	ws := windowStart.UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO icu_balances (workspace_id, window_start, total_icu, last_updated)
		 VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(workspace_id, window_start) DO UPDATE SET
		 total_icu = total_icu + EXCLUDED.total_icu,
		 last_updated = datetime('now')`,
		workspaceID, ws, delta,
	)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	return nil
}

func (s *Store) SaveSlot(ctx context.Context, slot SlotRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO active_slots (model_name, slot_id, host, port, cache_key, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(model_name, host, port) DO UPDATE SET
		 slot_id = EXCLUDED.slot_id,
		 cache_key = EXCLUDED.cache_key,
		 expires_at = EXCLUDED.expires_at,
		 last_used_at = datetime('now')`,
		slot.ModelName, slot.SlotID, slot.Host, slot.Port, slot.CacheKey, slot.ExpiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("save slot: %w", err)
	}
	return nil
}

func (s *Store) GetActiveSlot(ctx context.Context, modelName, host string, port int, cacheKey string) (*SlotRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT slot_id, host, port, cache_key, expires_at, last_used_at
		 FROM active_slots
		 WHERE model_name = ? AND host = ? AND port = ? AND cache_key = ? AND expires_at > datetime('now')`,
		modelName, host, port, cacheKey,
	)
	var slot SlotRecord
	slot.ModelName = modelName
	var expiresStr, lastUsedStr string
	err := row.Scan(&slot.SlotID, &slot.Host, &slot.Port, &slot.CacheKey, &expiresStr, &lastUsedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active slot: %w", err)
	}
	slot.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
	slot.LastUsedAt, _ = time.Parse("2006-01-02 15:04:05", lastUsedStr)
	return &slot, nil
}

func (s *Store) ExpireSlots(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM active_slots WHERE expires_at <= datetime('now')`)
	if err != nil {
		return fmt.Errorf("expire slots: %w", err)
	}
	return nil
}

func (s *Store) SetEntityMetadata(ctx context.Context, entityType, entityID, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO entity_metadata (entity_type, entity_id, key, value, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(entity_type, entity_id, key) DO UPDATE SET
		 value = EXCLUDED.value,
		 updated_at = datetime('now')`,
		entityType, entityID, key, value,
	)
	if err != nil {
		return fmt.Errorf("set entity metadata: %w", err)
	}
	return nil
}

func (s *Store) GetEntityMetadata(ctx context.Context, entityType, entityID, key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM entity_metadata WHERE entity_type = ? AND entity_id = ? AND key = ?`,
		entityType, entityID, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get entity metadata: %w", err)
	}
	return value, nil
}

func (s *Store) DeleteEntityMetadata(ctx context.Context, entityType, entityID, key string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM entity_metadata WHERE entity_type = ? AND entity_id = ? AND key = ?`,
		entityType, entityID, key,
	)
	if err != nil {
		return fmt.Errorf("delete entity metadata: %w", err)
	}
	return nil
}

func (s *Store) ListEntityMetadata(ctx context.Context, entityType, entityID string) (map[string][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM entity_metadata WHERE entity_type = ? AND entity_id = ?`,
		entityType, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("list entity metadata: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("list entity metadata scan: %w", err)
		}
		result[key] = value
	}
	return result, rows.Err()
}
