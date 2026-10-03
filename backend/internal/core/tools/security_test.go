package tools

import "testing"

func TestContainsSecret(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"the staging DB runs on port 5433", false},
		{"", false},
		{"key sk-" + "abcdefghijklmnopqrstuvwxyzABCDEF0123456789", true},
		{"AKIA" + "ABCDEFGHIJKLMNOP", true},
		{"use sk-short for tests", false}, // too short to be a key
	}
	for _, c := range cases {
		if got := ContainsSecret(c.in); got != c.want {
			t.Errorf("ContainsSecret(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
