package brief

import "testing"

func TestOutletKey(t *testing.T) {
	for in, want := range map[string]string{"The Guardian UK": "guardian", "Guardian.com": "guardian", "CNBC": "cnbc", "BBC News": "bbc news", "The UK": "uk", "  The  Guardian ": "guardian"} {
		if got := OutletKey(in); got != want {
			t.Errorf("OutletKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimilarKeys(t *testing.T) {
	same := [][2]string{
		{"nvidia board urged to resign after ceo reinstatement", "nvidia board urged to resign after ceo s reinstatement analysis"},
	}
	different := [][2]string{
		{"jensen huang is too big to fire putting nvidia s governance in focus analysts", "six key questions raised by jensen huang s return to nvidia"},
		{"uk disability benefit cuts risk worsening youth destitution", "uk interest rates held at four percent"},
		{"short one", "short one"},
	}
	for _, p := range same {
		if !SimilarKeys(p[0], p[1]) {
			t.Errorf("should be one story: %q / %q", p[0], p[1])
		}
	}
	for _, p := range different {
		if SimilarKeys(p[0], p[1]) {
			t.Errorf("should be two stories: %q / %q", p[0], p[1])
		}
	}
}
