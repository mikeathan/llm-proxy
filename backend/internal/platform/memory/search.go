package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// searchHotSQL fetches entries tagged "hot" — these are mode:"always" entries
// injected at session start. Two branches cover both scopes:
//   - workspace_id = 'global' → user-scope facts (applies to all projects)
//   - workspace_id = ?        → workspace-scope facts (this project only)
// Uses json_each for exact tag matching (not FTS5). Ordered by recency so the
// most recently saved facts appear first in the prompt.
const searchHotSQL = `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content,
                             m.tags, m.source, m.created_at, m.updated_at
                      FROM memories m
                      WHERE (m.workspace_id = 'global'
                             AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
                         OR (m.workspace_id = ?
                             AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
                      ORDER BY m.updated_at DESC`

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
//
// If opt.WorkspaceID is set, it overrides the workspace filter (e.g. for
// global-scope user searches).
func (s *Store) Search(ctx context.Context, workspaceID, query string, limit int, opts ...SearchOption) ([]MemoryEntry, error) {
	opt := SearchOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	filterWSID := workspaceID
	if opt.WorkspaceID != "" {
		filterWSID = opt.WorkspaceID
	}

	switch {
	case query != "" && len(opt.Tags) > 0:
		return s.searchFTSandTags(ctx, filterWSID, sanitiseFTSQuery(query), opt.Tags, limit, opt.SearchAllWorkspaces)
	case query == "" && len(opt.Tags) > 0:
		return s.searchByTagsOnly(ctx, filterWSID, opt.Tags, limit, opt.SearchAllWorkspaces)
	case query != "":
		return s.searchFTSonly(ctx, filterWSID, sanitiseFTSQuery(query), opt.MemType, limit, opt.SearchAllWorkspaces)
	default:
		return []MemoryEntry{}, nil
	}
}

// SearchHot fetches all entries tagged with "hot" — these are the mode:"always"
// entries that get injected into every session.  Global entries (workspace_id =
// 'global') are included alongside entries for the given workspace.
func (s *Store) SearchHot(ctx context.Context, wsID string) ([]MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, searchHotSQL, wsID)
	if err != nil {
		return nil, fmt.Errorf("memory search hot: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) searchFTSandTags(ctx context.Context, workspaceID, sanitised string, tags []string, limit int, allWorkspaces bool) ([]MemoryEntry, error) {
	q := workspaceQuery(searchFTSandTagsSQL, allWorkspaces)
	q, tagArgs := tagClause(tags, q)
	q += " ORDER BY rank LIMIT ?"
	args := append([]any{workspaceID, sanitised}, tagArgs...)
	args = append(args, limit)
	return s.queryRows(ctx, q, args...)
}

func (s *Store) searchByTagsOnly(ctx context.Context, workspaceID string, tags []string, limit int, allWorkspaces bool) ([]MemoryEntry, error) {
	q := workspaceQuery(searchTagsOnlySQL, allWorkspaces)
	q, tagArgs := tagClause(tags, q)
	q += " ORDER BY m.updated_at DESC LIMIT ?"
	args := append([]any{workspaceID}, tagArgs...)
	args = append(args, limit)
	return s.queryRows(ctx, q, args...)
}

func (s *Store) searchFTSonly(ctx context.Context, workspaceID, sanitised string, memType MemoryType, limit int, allWorkspaces bool) ([]MemoryEntry, error) {
	if memType != "" {
		q := workspaceQuery(searchTagsOnlySQL, allWorkspaces)
		q += ` AND memories_fts MATCH ? AND m.memory_type = ? ORDER BY rank LIMIT ?`
		return s.queryRows(ctx, q, workspaceID, sanitised, string(memType), limit)
	}
	q := workspaceQuery(searchFTSandTagsSQL, allWorkspaces)
	q += ` ORDER BY rank LIMIT ?`
	return s.queryRows(ctx, q, workspaceID, sanitised, limit)
}

// workspaceQuery injects the workspace_id filter into a %s SQL template.
// When allWorkspaces is true, it matches both the workspace and global scope,
// so unscoped search returns user-scope facts alongside workspace-scope facts.
func workspaceQuery(tmpl string, allWorkspaces bool) string {
	if allWorkspaces {
		return fmt.Sprintf(tmpl, "(m.workspace_id = ? OR m.workspace_id = 'global')")
	}
	return fmt.Sprintf(tmpl, "m.workspace_id = ?")
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
