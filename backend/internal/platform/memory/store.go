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

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts4(
    title, content, tokenize=unicode61, content=memories
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(docid, title, content)
    VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    DELETE FROM memories_fts WHERE docid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    DELETE FROM memories_fts WHERE docid = old.id;
    INSERT INTO memories_fts(docid, title, content)
    VALUES (new.id, new.title, new.content);
END;
`

type Store struct {
	db *sql.DB
}

func New(p db.Provider) (*Store, error) {
	database := p.DB()
	if _, err := database.Exec(migrateSQL); err != nil {
		return nil, fmt.Errorf("memory migrate: %w", err)
	}
	return &Store{db: database}, nil
}

func (s *Store) Insert(ctx context.Context, workspaceID string, memoryType MemoryType, title, content, source string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (workspace_id, memory_type, title, content, source)
		 VALUES (?, ?, ?, ?, ?)`,
		workspaceID, string(memoryType), title, content, source,
	)
	if err != nil {
		return 0, fmt.Errorf("memory insert: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) Search(ctx context.Context, workspaceID, query string, limit int) ([]MemoryEntry, error) {
	if query == "" {
		return nil, nil
	}

	sanitised := sanitiseFTSQuery(query)

	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.source, m.created_at, m.updated_at
		 FROM memories m
		 JOIN memories_fts f ON m.id = f.docid
		 WHERE m.workspace_id = ? AND memories_fts MATCH ?
		 ORDER BY f.docid DESC
		 LIMIT ?`,
		workspaceID, sanitised, limit,
	)
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
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workspace_id, memory_type, title, content, source, created_at, updated_at
			 FROM memories
			 WHERE workspace_id = ? AND memory_type = ?
			 ORDER BY updated_at DESC
			 LIMIT ? OFFSET ?`,
			workspaceID, string(memoryType), limit, offset,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workspace_id, memory_type, title, content, source, created_at, updated_at
			 FROM memories
			 WHERE workspace_id = ?
			 ORDER BY updated_at DESC
			 LIMIT ? OFFSET ?`,
			workspaceID, limit, offset,
		)
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
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, memory_type, title, content, source, created_at, updated_at
		 FROM memories
		 WHERE workspace_id = ? AND id = ?`,
		workspaceID, id,
	).Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title, &e.Content, &e.Source, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory get: %w", err)
	}
	return &e, nil
}

func (s *Store) Update(ctx context.Context, workspaceID string, id int64, title, content string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET title = ?, content = ?, updated_at = datetime('now')
		 WHERE workspace_id = ? AND id = ?`,
		title, content, workspaceID, id,
	)
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
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM memories WHERE workspace_id = ? AND id = ?`,
		workspaceID, id,
	)
	if err != nil {
		return fmt.Errorf("memory delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory delete: not found")
	}
	return nil
}

func (s *Store) DeleteOlderThan(ctx context.Context, memoryType MemoryType, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM memories WHERE memory_type = ? AND created_at < ?`,
		string(memoryType), before.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("memory delete older than: %w", err)
	}
	return res.RowsAffected()
}

// sanitiseFTSQuery removes characters that would cause FTS MATCH syntax errors.
// Splits on whitespace and joins terms with OR so that any matching word
// returns a result (default AND semantics are too restrictive for search).
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
	return strings.Join(terms, " OR ")
}
