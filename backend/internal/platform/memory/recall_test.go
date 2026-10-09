package memory

import (
	"context"
	"testing"
)

// RecallQuery keeps only the words of a chat message that carry meaning, so a
// generic question does not OR-match every stored fact.
func TestRecallQuery(t *testing.T) {
	for _, tc := range []struct {
		name, message, want string
	}{
		{"keeps the topic word", "Which database does the project use today?", "database today"},
		{"drops question words and pronouns", "What is my preferred editor?", "preferred editor"},
		{"possessive fragment dropped", "What is this project's codename?", "codename"},
		{"case and duplicates folded", "Database DATABASE database", "database"},
		{"only generic words -> no recall", "Can you help me with this project please?", ""},
		{"punctuation only -> no recall", "?!", ""},
		{"empty", "", ""},
		{"digits kept", "Why does port 4001 fail?", "port 4001 fail"},
		{"capped term count", "alpha bravo charlie delta echo foxtrot golf hotel india juliet", "alpha bravo charlie delta echo foxtrot golf hotel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RecallQuery(tc.message); got != tc.want {
				t.Errorf("RecallQuery(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}

// Recall precision against a seeded store: a topical question finds its fact,
// a generic project request and a paraphrase without shared words find nothing.
func TestRecallQuery_AgainstStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seed := map[string]string{
		"codename": "The project's codename is BLUEHERON.",
		"editor":   "The user's preferred code editor is Zed.",
		"database": "Since September 2026 the project database is SQLite.",
	}
	ids := map[string]int64{}
	for k, content := range seed {
		id, err := s.Insert(ctx, "ws", LongTerm, k, content, nil, "test")
		if err != nil {
			t.Fatal(err)
		}
		ids[k] = id
	}

	search := func(message string) []int64 {
		q := RecallQuery(message)
		if q == "" {
			return nil
		}
		got, err := s.Search(ctx, "ws", q, 3, SearchOption{SearchAllWorkspaces: true})
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, e := range got {
			out = append(out, e.ID)
		}
		return out
	}

	for _, tc := range []struct {
		message string
		want    []int64
	}{
		{"Which database does the project use today?", []int64{ids["database"]}},
		{"What is my preferred editor?", []int64{ids["editor"]}},
		{"What is this project's codename?", []int64{ids["codename"]}},
		{"Can you help me tidy up this project's README?", nil}, // generic project words recall nothing
		{"What's our DB?", nil}, // documented limit: no shared word
	} {
		got := search(tc.message)
		if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
			t.Errorf("recall for %q = %v, want %v", tc.message, got, tc.want)
		}
	}
}
