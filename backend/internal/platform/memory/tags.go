package memory

import (
	"encoding/json"
	"sort"
	"strings"
)

// normalizeTags lowercases, trims, sorts and deduplicates a tag slice.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		n := strings.TrimSpace(strings.ToLower(t))
		if n != "" && !seen[n] {
			seen[n] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// tagsJSON serialises tags to a JSON array for SQL storage.
func tagsJSON(tags []string) string {
	b, _ := json.Marshal(normalizeTags(tags))
	return string(b)
}

// mergeTags returns the sorted, deduplicated union of two tag slices.
func mergeTags(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for _, t := range a {
		seen[t] = true
	}
	for _, t := range b {
		seen[t] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// UpdateTags controls whether Update replaces tags or merges them with existing.
type UpdateTags int

const (
	ReplaceTags UpdateTags = iota
	MergeTags
)

// tagClause appends AND EXISTS clauses for each tag and returns the args.
// Used by Search to build the tag-filtered query dynamically.
func tagClause(tags []string, query string) (string, []any) {
	if len(tags) == 0 {
		return query, nil
	}
	var b strings.Builder
	b.WriteString(query)
	var args []any
	for _, t := range normalizeTags(tags) {
		b.WriteString(" AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = ?)")
		args = append(args, t)
	}
	return b.String(), args
}
