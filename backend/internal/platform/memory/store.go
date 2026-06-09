package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/platform/db"
)

// ──────────────────────────────────────────────────────────────
//  Schema & queries
// ──────────────────────────────────────────────────────────────

const migrateSQL = `
CREATE TABLE IF NOT EXISTS memories (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    memory_type  TEXT NOT NULL DEFAULT 'long_term',
    title        TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL,
    tags         TEXT NOT NULL DEFAULT '[]',
    source       TEXT NOT NULL DEFAULT 'agent',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_memories_ws ON memories(workspace_id, memory_type);

-- Drop triggers from any previous FTS4 schema (they reference docid, not rowid).
-- Must happen before the table DROP or SQLite may reject it.
DROP TRIGGER IF EXISTS memories_ai;
DROP TRIGGER IF EXISTS memories_ad;
DROP TRIGGER IF EXISTS memories_au;

-- Drop any existing FTS table (migration from FTS4 to FTS5 or tags column).
DROP TABLE IF EXISTS memories_fts;

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    title, content, tags, tokenize='unicode61', content=memories, content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
    INSERT INTO memories_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;
`

// rebuildFTSIndexSQL repopulates the FTS index from existing rows after a
// DROP/CREATE migration. The sync triggers keep the index current for any
// future INSERT/UPDATE/DELETE, but existing rows need this initial fill.
const rebuildFTSIndexSQL = `INSERT INTO memories_fts(rowid, title, content, tags) SELECT id, title, content, tags FROM memories`

// insertSQL inserts a new memory entry with all required fields including tags.
const insertSQL = `INSERT INTO memories (workspace_id, memory_type, title, content, tags, source) VALUES (?, ?, ?, ?, ?, ?)`

// searchFTSandTagsSQL performs FTS5 full-text search with optional tag filter
// (EXISTS clause appended dynamically). Ordered by BM25 relevance (rank ASC).
const searchFTSandTagsSQL = `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.tags, m.source, m.created_at, m.updated_at
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE m.workspace_id = ? AND memories_fts MATCH ?`

// searchTagsOnlySQL lists entries filtered by tag without an FTS query.
// Used when the agent provides tags but no text query.
const searchTagsOnlySQL = `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.tags, m.source, m.created_at, m.updated_at
FROM memories m
WHERE m.workspace_id = ?`

// selectColumnsSQL is the shared column projection used by list/get/find queries,
// now including the tags column.
const selectColumnsSQL = `SELECT id, workspace_id, memory_type, title, content, tags, source, created_at, updated_at`

// listByTypeSQL lists memories filtered by type, most recently updated first.
const listByTypeSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND memory_type = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`

// listSQL lists all memories for a workspace, most recently updated first.
const listSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`

// getSQL fetches a single memory entry by ID within a workspace.
const getSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND id = ?`

// updateSQL updates the title, content, and tags of a memory entry and bumps its timestamp.
const updateSQL = `UPDATE memories SET title = ?, content = ?, tags = ?, updated_at = datetime('now') WHERE workspace_id = ? AND id = ?`

// deleteSQL removes a single memory entry by ID within a workspace.
const deleteSQL = `DELETE FROM memories WHERE workspace_id = ? AND id = ?`

// existsSQL checks whether a memory with the exact content already exists in the workspace.
const existsSQL = `SELECT COUNT(*) FROM memories WHERE workspace_id = ? AND content = ?`

// findByTitleSQL finds the first memory entry with the given title in a workspace.
const findByTitleSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND title = ? LIMIT 1`

// findSubstringSQL locates the single memory entry containing a content substring.
const findSubstringSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND content LIKE ?`

// charCountSQL returns the total character count of all memory content in a workspace.
const charCountSQL = `SELECT COALESCE(SUM(LENGTH(content)), 0) FROM memories WHERE workspace_id = ?`

// deleteAllWorkspaceSQL removes all memories for a workspace (or filtered by type).
const deleteAllWorkspaceSQL = `DELETE FROM memories WHERE workspace_id = ?`
const deleteByTypeWorkspaceSQL = `DELETE FROM memories WHERE workspace_id = ? AND memory_type = ?`

// deleteOlderThanSQL removes memories of a given type created before a cutoff timestamp.
const deleteOlderThanSQL = `DELETE FROM memories WHERE memory_type = ? AND created_at < ?`

// sqliteDateTimeFormat is the standard SQLite datetime function format.
const sqliteDateTimeFormat = "2006-01-02 15:04:05"

// ──────────────────────────────────────────────────────────────
//  Store
// ──────────────────────────────────────────────────────────────

type Store struct {
	db *sql.DB
}

func New(p db.Provider) (*Store, error) {
	database := p.DB()
	if _, err := database.Exec(migrateSQL); err != nil {
		return nil, fmt.Errorf("memory migrate: %w", err)
	}
	// Idempotent migration — SQLite doesn't support IF NOT EXISTS for ALTER TABLE.
	if _, err := database.Exec(`ALTER TABLE memories ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("memory migrate tags: %w", err)
		}
	}
	// Rebuild FTS index for entries that existed before the drop/create above.
	// The triggers will keep the index in sync going forward, but existing rows
	// need an initial population.
	if _, err := database.Exec(rebuildFTSIndexSQL); err != nil {
		return nil, fmt.Errorf("memory fts rebuild: %w", err)
	}
	return &Store{db: database}, nil
}

// ──────────────────────────────────────────────────────────────
//  CRUD
// ──────────────────────────────────────────────────────────────

func (s *Store) Insert(ctx context.Context, workspaceID string, memoryType MemoryType, title, content string, tags []string, source string) (int64, error) {
	res, err := s.db.ExecContext(ctx, insertSQL, workspaceID, string(memoryType), title, content, tagsJSON(tags), source)
	if err != nil {
		return 0, fmt.Errorf("memory insert: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) Get(ctx context.Context, workspaceID string, id int64) (*MemoryEntry, error) {
	e, err := scanMemoryEntry(s.db.QueryRowContext(ctx, getSQL, workspaceID, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory get: %w", err)
	}
	return &e, nil
}

func (s *Store) Update(ctx context.Context, workspaceID string, id int64, title, content string, tags []string, tagMode UpdateTags) error {
	normalized := normalizeTags(tags)

	if tagMode == MergeTags {
		existing, err := s.Get(ctx, workspaceID, id)
		if err != nil {
			return fmt.Errorf("memory update get: %w", err)
		}
		if existing != nil {
			normalized = mergeTags(existing.Tags, normalized)
		}
	}

	res, err := s.db.ExecContext(ctx, updateSQL, title, content, tagsJSON(normalized), workspaceID, id)
	if err != nil {
		return fmt.Errorf("memory update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory update: not found")
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, workspaceID string, id int64) error {
	res, err := s.db.ExecContext(ctx, deleteSQL, workspaceID, id)
	if err != nil {
		return fmt.Errorf("memory delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory delete: not found")
	}
	return nil
}

func (s *Store) Exists(ctx context.Context, workspaceID, content string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, existsSQL, workspaceID, content).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("memory exists check: %w", err)
	}
	return count > 0, nil
}

// ──────────────────────────────────────────────────────────────
//  List
// ──────────────────────────────────────────────────────────────

func (s *Store) List(ctx context.Context, workspaceID string, memoryType MemoryType, limit, offset int) ([]MemoryEntry, error) {
	var query string
	var args []any
	if memoryType != "" {
		query = listByTypeSQL
		args = []any{workspaceID, string(memoryType), limit, offset}
	} else {
		query = listSQL
		args = []any{workspaceID, limit, offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory list: %w", err)
	}
	defer rows.Close()

	entries, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("memory list scan: %w", err)
	}
	return entries, nil
}

// ──────────────────────────────────────────────────────────────
//  Lookup helpers
// ──────────────────────────────────────────────────────────────

func (s *Store) FindByTitle(ctx context.Context, workspaceID, title string) (*MemoryEntry, error) {
	e, err := scanMemoryEntry(s.db.QueryRowContext(ctx, findByTitleSQL, workspaceID, title))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory find by title: %w", err)
	}
	return &e, nil
}

func (s *Store) FindByContentSubstring(ctx context.Context, workspaceID, substr string) (*MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, findSubstringSQL, workspaceID, "%"+substr+"%")
	if err != nil {
		return nil, fmt.Errorf("memory find substring: %w", err)
	}
	defer rows.Close()

	entries, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("memory find substring scan: %w", err)
	}

	switch len(entries) {
	case 0:
		return nil, fmt.Errorf("no memory entry matching %q", substr)
	case 1:
		return &entries[0], nil
	default:
		return nil, fmt.Errorf("substring %q matches %d entries — be more specific", substr, len(entries))
	}
}

func (s *Store) WorkspaceCharCount(ctx context.Context, workspaceID string) (int, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx, charCountSQL, workspaceID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("memory char count: %w", err)
	}
	return int(total.Int64), nil
}

// ──────────────────────────────────────────────────────────────
//  Bulk delete
// ──────────────────────────────────────────────────────────────

func (s *Store) DeleteAllByWorkspace(ctx context.Context, workspaceID string, memoryType MemoryType) (int64, error) {
	var err error
	var res sql.Result
	if memoryType != "" {
		res, err = s.db.ExecContext(ctx, deleteByTypeWorkspaceSQL, workspaceID, string(memoryType))
	} else {
		res, err = s.db.ExecContext(ctx, deleteAllWorkspaceSQL, workspaceID)
	}
	if err != nil {
		return 0, fmt.Errorf("memory delete all: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) DeleteOlderThan(ctx context.Context, memoryType MemoryType, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, deleteOlderThanSQL, string(memoryType), before.UTC().Format(sqliteDateTimeFormat))
	if err != nil {
		return 0, fmt.Errorf("memory delete older than: %w", err)
	}
	return res.RowsAffected()
}
