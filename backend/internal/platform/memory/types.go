// Package memory provides a SQLite-backed persistent memory store for
// agent facts, decisions, and preferences.  It uses FTS5 for full-text
// search and shares the same database file as the ledger package.
package memory

import "time"

type MemoryType string

const (
	LongTerm MemoryType = "long_term"
	Daily    MemoryType = "daily"
	Session  MemoryType = "session"
)

// MemoryEntry stores datetime as string to match go-sqlite3 scan behavior.
// The existing ledger code (store.go:233-242) follows the same pattern:
// scan DATETIME columns as string, parse with time.Parse when needed.
type MemoryEntry struct {
	ID          int64      `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	MemoryType  MemoryType `json:"memory_type"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Source      string     `json:"source"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func (e *MemoryEntry) CreatedAtTime() (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", e.CreatedAt)
}

func (e *MemoryEntry) UpdatedAtTime() (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", e.UpdatedAt)
}
