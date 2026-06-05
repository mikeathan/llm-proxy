// Package memory provides a SQLite-backed persistent memory store for
// agent facts, decisions, and preferences.  It uses FTS5 for full-text
// search and shares the same database file as the ledger package.
package memory

import (
	"fmt"
	"time"
)

type MemoryType string

const (
	LongTerm    MemoryType = "long_term"
	Daily       MemoryType = "daily"
	Session     MemoryType = "session"
	UserProfile MemoryType = "user_profile"
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
	return parseMemoryTime(e.CreatedAt)
}

func (e *MemoryEntry) UpdatedAtTime() (time.Time, error) {
	return parseMemoryTime(e.UpdatedAt)
}

// parseMemoryTime handles both SQLite datetime('now') format
// (2006-01-02 15:04:05) and ISO 8601 (2006-01-02T15:04:05Z).
func parseMemoryTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: SQLite and RFC3339 both failed", s)
}
