package memory

import (
	"slices"
	"testing"
)

func TestWithHot(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		hot  bool
		want []string
	}{
		{"add to empty", nil, true, []string{HotTag}},
		{"add keeps others, sorted", []string{"ci"}, true, []string{"ci", HotTag}},
		{"add is idempotent", []string{HotTag}, true, []string{HotTag}},
		{"remove keeps others", []string{HotTag, "ci"}, false, []string{"ci"}},
		{"remove from none", nil, false, []string{}},
		{"matches case-insensitively when removing", []string{"HOT"}, false, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithHot(tc.in, tc.hot); !slices.Equal(got, tc.want) {
				t.Errorf("WithHot(%v, %v) = %v, want %v", tc.in, tc.hot, got, tc.want)
			}
		})
	}
}

func TestMemoryEntry_IsHot(t *testing.T) {
	if !(MemoryEntry{Tags: []string{"ci", HotTag}}).IsHot() {
		t.Error("an entry tagged hot is hot")
	}
	if (MemoryEntry{Tags: []string{"ci"}}).IsHot() {
		t.Error("an entry without the hot tag is not hot")
	}
}
