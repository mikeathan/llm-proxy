package memory

import "strings"

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
