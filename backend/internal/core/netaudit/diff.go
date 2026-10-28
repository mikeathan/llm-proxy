package netaudit

import "slices"

// Change is a finding whose key stayed the same but whose version text did not.
type Change struct {
	Before Finding `json:"before"`
	After  Finding `json:"after"`
}

// Delta is how one scan differs from the one before it.
type Delta struct {
	New       []Finding `json:"new"`
	Resolved  []Finding `json:"resolved"`
	Changed   []Change  `json:"changed"`
	Unchanged int       `json:"unchanged"`
}

// Diff compares the previous findings with the current ones by Key. Every list is sorted by key so the same two scans
// always give the same Delta. A key repeated within one side counts once, the last wins.
func Diff(prev, cur []Finding) Delta {
	before, after := byKey(prev), byKey(cur)
	var d Delta
	for _, key := range sortedKeys(after) {
		now := after[key]
		was, existed := before[key]
		switch {
		case !existed:
			d.New = append(d.New, now)
		case was.Version != now.Version || !slices.Equal(was.CVEs, now.CVEs) || !slices.Equal(was.Exploited, now.Exploited):
			d.Changed = append(d.Changed, Change{Before: was, After: now})
		default:
			d.Unchanged++
		}
	}
	for _, key := range sortedKeys(before) {
		if _, still := after[key]; !still {
			d.Resolved = append(d.Resolved, before[key])
		}
	}
	return d
}

func byKey(findings []Finding) map[string]Finding {
	out := make(map[string]Finding, len(findings))
	for _, f := range findings {
		out[f.Key()] = f
	}
	return out
}

func sortedKeys(m map[string]Finding) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Empty reports whether nothing changed.
func (d Delta) Empty() bool {
	return len(d.New) == 0 && len(d.Resolved) == 0 && len(d.Changed) == 0
}
