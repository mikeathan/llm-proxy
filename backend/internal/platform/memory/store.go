package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/platform/db"
)

const migrateSQL = `
CREATE TABLE IF NOT EXISTS memories (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    memory_type  TEXT NOT NULL DEFAULT 'long_term',
    title        TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL,
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

-- Drop any existing FTS table (migration from FTS4 to FTS5).
DROP TABLE IF EXISTS memories_fts;

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    title, content, tokenize='unicode61', content=memories, content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, title, content)
    VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content)
    VALUES ('delete', old.id, old.title, old.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content)
    VALUES ('delete', old.id, old.title, old.content);
    INSERT INTO memories_fts(rowid, title, content)
    VALUES (new.id, new.title, new.content);
END;
`

// rebuildFTSIndexSQL repopulates the FTS index from existing rows after a
// DROP/CREATE migration. The sync triggers keep the index current for any
// future INSERT/UPDATE/DELETE, but existing rows need this initial fill.
const rebuildFTSIndexSQL = `INSERT INTO memories_fts(rowid, title, content) SELECT id, title, content FROM memories`

// insertSQL inserts a new memory entry with all required fields.
const insertSQL = `INSERT INTO memories (workspace_id, memory_type, title, content, source) VALUES (?, ?, ?, ?, ?)`

// searchSQL performs FTS5 full-text search across title and content columns,
// ordered by BM25 relevance (rank ASC = most relevant first).
const searchSQL = `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.source, m.created_at, m.updated_at
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE m.workspace_id = ? AND memories_fts MATCH ?
ORDER BY rank
LIMIT ?`

// searchByTypeSQL is searchSQL with an additional memory_type filter.
const searchByTypeSQL = `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.source, m.created_at, m.updated_at
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE m.workspace_id = ? AND memories_fts MATCH ? AND m.memory_type = ?
ORDER BY rank
LIMIT ?`

// selectColumnsSQL is the shared column projection used by list/get/find queries.
const selectColumnsSQL = `SELECT id, workspace_id, memory_type, title, content, source, created_at, updated_at`

// listByTypeSQL lists memories filtered by type, most recently updated first.
const listByTypeSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND memory_type = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`

// listSQL lists all memories for a workspace, most recently updated first.
const listSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`



// getSQL fetches a single memory entry by ID within a workspace.
const getSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND id = ?`

// updateSQL updates the title and content of a memory entry and bumps its timestamp.
const updateSQL = `UPDATE memories SET title = ?, content = ?, updated_at = datetime('now') WHERE workspace_id = ? AND id = ?`

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

type Store struct {
	db *sql.DB
}

func New(p db.Provider) (*Store, error) {
	database := p.DB()
	if _, err := database.Exec(migrateSQL); err != nil {
		return nil, fmt.Errorf("memory migrate: %w", err)
	}
	// Rebuild FTS index for entries that existed before the drop/create above.
	// The triggers will keep the index in sync going forward, but existing rows
	// need an initial population.
	if _, err := database.Exec(rebuildFTSIndexSQL); err != nil {
		return nil, fmt.Errorf("memory fts rebuild: %w", err)
	}
	return &Store{db: database}, nil
}

func (s *Store) Insert(ctx context.Context, workspaceID string, memoryType MemoryType, title, content, source string) (int64, error) {
	res, err := s.db.ExecContext(ctx, insertSQL, workspaceID, string(memoryType), title, content, source)
	if err != nil {
		return 0, fmt.Errorf("memory insert: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) Search(ctx context.Context, workspaceID, query string, limit int, memoryType ...MemoryType) ([]MemoryEntry, error) {
	if query == "" {
		return nil, nil
	}

	sanitised := sanitiseFTSQuery(query)

	var rows *sql.Rows
	var err error
	if len(memoryType) > 0 {
		rows, err = s.db.QueryContext(ctx, searchByTypeSQL, workspaceID, sanitised, string(memoryType[0]), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, searchSQL, workspaceID, sanitised, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("memory search: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory search scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory search rows: %w", err)
	}
	return entries, nil
}

func (s *Store) List(ctx context.Context, workspaceID string, memoryType MemoryType, limit, offset int) ([]MemoryEntry, error) {
	var rows *sql.Rows
	var err error

	if memoryType != "" {
		rows, err = s.db.QueryContext(ctx, listByTypeSQL, workspaceID, string(memoryType), limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx, listSQL, workspaceID, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("memory list: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory list scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory list rows: %w", err)
	}
	return entries, nil
}

func (s *Store) Get(ctx context.Context, workspaceID string, id int64) (*MemoryEntry, error) {
	var e MemoryEntry
	err := s.db.QueryRowContext(ctx, getSQL, workspaceID, id).
		Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory get: %w", err)
	}
	return &e, nil
}

func (s *Store) Update(ctx context.Context, workspaceID string, id int64, title, content string) error {
	res, err := s.db.ExecContext(ctx, updateSQL, title, content, workspaceID, id)
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

// FindByTitle returns the first memory entry with the given title, or nil if none exists.
// Used by memory_update to deduplicate entries by topic across runs.
func (s *Store) FindByTitle(ctx context.Context, workspaceID, title string) (*MemoryEntry, error) {
	var e MemoryEntry
	err := s.db.QueryRowContext(ctx, findByTitleSQL, workspaceID, title).
		Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory find by title: %w", err)
	}
	return &e, nil
}

// FindByContentSubstring returns the single memory entry containing substr.
// Returns an error if zero or multiple entries match — designed for
// memory_update old_text where the agent targets a unique entry.
func (s *Store) FindByContentSubstring(ctx context.Context, workspaceID, substr string) (*MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, findSubstringSQL, workspaceID, "%"+substr+"%")
	if err != nil {
		return nil, fmt.Errorf("memory find substring: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory find substring scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory find substring rows: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no memory entry matching %q", substr)
	}
	if len(entries) > 1 {
		return nil, fmt.Errorf("substring %q matches %d entries — be more specific", substr, len(entries))
	}
	return &entries[0], nil
}

func (s *Store) WorkspaceCharCount(ctx context.Context, workspaceID string) (int, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx, charCountSQL, workspaceID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("memory char count: %w", err)
	}
	return int(total.Int64), nil
}

// DeleteAllByWorkspace removes all memories for a workspace. When memoryType is
// non-empty only entries of that type are deleted. Returns the count of deleted rows.
func (s *Store) DeleteAllByWorkspace(ctx context.Context, workspaceID string, memoryType MemoryType) (int64, error) {
	var res sql.Result
	var err error
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

// sanitiseFTSQuery removes characters that would cause FTS MATCH syntax errors.
// Splits on whitespace and joins terms with OR so that any matching word
// returns a result (default AND semantics are too restrictive for search).
// isFTSStopWord returns true for words that are common in task-file structure
// headings but carry no semantic value for FTS5 content search. Filtering them
// out improves BM25 ranking of task-relevant memory entries.
func isFTSStopWord(w string) bool {
	switch w {
	case "step", "task", "run", "use", "check", "the", "a", "an",
		"and", "or", "to", "in", "of", "for", "with", "on", "by",
		"at", "is", "be", "do", "not", "are", "was", "will", "can":
		return true
	}
	return false
}

func sanitiseFTSQuery(q string) string {
	var b strings.Builder
	b.Grow(len(q))
	inSpace := false
	for _, r := range q {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			if r == ' ' {
				if !inSpace {
					b.WriteRune(' ')
					inSpace = true
				}
			} else {
				b.WriteRune(r)
				inSpace = false
			}
		} else {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" {
		return cleaned
	}
	terms := strings.Fields(cleaned)

	// Filter out task-structure stop words that would pollute BM25 ranking.
	// Words like "step", "run", "use" appear in every step heading and inflate
	// matches against generic memory entries that happen to share these words.
	filtered := make([]string, 0, len(terms))
	for _, t := range terms {
		if !isFTSStopWord(t) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		return cleaned
	}

	quoted := make([]string, len(filtered))
	for i, t := range filtered {
		quoted[i] = `"` + t + `"`
	}
	return strings.Join(quoted, " OR ")
}
