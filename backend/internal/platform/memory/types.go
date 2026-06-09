// Package memory provides a SQLite-backed persistent memory store for
// agent facts, decisions, and preferences.  It uses FTS5 for full-text
// search and shares the same database file as the ledger package.
package memory

import (
	"encoding/json"
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

// SearchOption carries optional filters for Store.Search(). Tags filters by
// exact tag match (all must be present). MemType replaces the old variadic.
type SearchOption struct {
	Tags    []string
	MemType MemoryType
}

// MemoryEntry stores datetime as string to match go-sqlite3 scan behavior.
// The existing ledger code (store.go:233-242) follows the same pattern:
// scan DATETIME columns as string, parse with time.Parse when needed.
type MemoryEntry struct {
	ID          int64      `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	MemoryType  MemoryType `json:"memory_type"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Tags        []string   `json:"tags"`
	Source      string     `json:"source"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// scanMemoryEntry is a shared row scanner for all Store methods to avoid
// duplicating the 9-column Scan call at every query site.
func scanMemoryEntry(row interface{ Scan(dest ...any) error }) (MemoryEntry, error) {
	var e MemoryEntry
	var tagsStr string
	if err := row.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title,
		&e.Content, &tagsStr, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return MemoryEntry{}, err
	}
	if tagsStr != "" && tagsStr != "[]" {
		json.Unmarshal([]byte(tagsStr), &e.Tags)
	}
	return e, nil
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
