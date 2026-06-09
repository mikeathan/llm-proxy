package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// scanRows iterates rows, scans each into MemoryEntry via scanMemoryEntry,
// and returns the slice. The caller must still close rows on error.
func scanRows(rows *sql.Rows) ([]MemoryEntry, error) {
	var entries []MemoryEntry
	for rows.Next() {
		e, err := scanMemoryEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Search returns memory entries matching the query and/or tags.
// Four query variants depending on inputs:
//   - query + tags: FTS5 full-text search filtered by tags (AND)
//   - tags only: returns entries with any matching tag, no text search
//   - query only: backward-compatible FTS5 search
//   - neither: returns empty slice
func (s *Store) Search(ctx context.Context, workspaceID, query string, limit int, opts ...SearchOption) ([]MemoryEntry, error) {
	opt := SearchOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	switch {
	case query != "" && len(opt.Tags) > 0:
		return s.searchFTSandTags(ctx, workspaceID, sanitiseFTSQuery(query), opt.Tags, limit)
	case query == "" && len(opt.Tags) > 0:
		return s.searchByTagsOnly(ctx, workspaceID, opt.Tags, limit)
	case query != "":
		return s.searchFTSonly(ctx, workspaceID, sanitiseFTSQuery(query), opt.MemType, limit)
	default:
		return []MemoryEntry{}, nil
	}
}

func (s *Store) searchFTSandTags(ctx context.Context, workspaceID, sanitised string, tags []string, limit int) ([]MemoryEntry, error) {
	q, tagArgs := tagClause(tags, searchFTSandTagsSQL)
	q += " ORDER BY rank LIMIT ?"
	args := append([]any{workspaceID, sanitised}, tagArgs...)
	args = append(args, limit)
	return s.queryRows(ctx, q, args...)
}

func (s *Store) searchByTagsOnly(ctx context.Context, workspaceID string, tags []string, limit int) ([]MemoryEntry, error) {
	q, tagArgs := tagClause(tags, searchTagsOnlySQL)
	q += " ORDER BY m.updated_at DESC LIMIT ?"
	args := append([]any{workspaceID}, tagArgs...)
	args = append(args, limit)
	return s.queryRows(ctx, q, args...)
}

func (s *Store) searchFTSonly(ctx context.Context, workspaceID, sanitised string, memType MemoryType, limit int) ([]MemoryEntry, error) {
	if memType != "" {
		q := searchTagsOnlySQL + ` AND memories_fts MATCH ? AND m.memory_type = ? ORDER BY rank LIMIT ?`
		return s.queryRows(ctx, q, workspaceID, sanitised, string(memType), limit)
	}
	q := searchFTSandTagsSQL + ` ORDER BY rank LIMIT ?`
	return s.queryRows(ctx, q, workspaceID, sanitised, limit)
}

// queryRows executes a QueryContext, scans the result via scanRows, and
// handles error wrapping. The caller supplies a complete query with args.
func (s *Store) queryRows(ctx context.Context, query string, args ...any) ([]MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory search: %w", err)
	}
	defer rows.Close()

	entries, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("memory search scan: %w", err)
	}
	return entries, nil
}
