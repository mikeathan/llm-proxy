package vulnmatch

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b   string
		want   int
		wantOK bool
	}{
		{"1.0", "1.0", 0, true},
		{"1.0", "1.0.0", 0, true},
		{"1.2", "1.10", -1, true},
		{"2.0", "1.99", 1, true},
		{"10.0", "9.9", 1, true},
		{"1.0.0001", "1.0.1", 0, true},
		{"1.0.2k", "1.0.2", 1, true},
		{"1.0.2", "1.0.2k", -1, true},
		{"1.0.2k", "1.0.2m", -1, true},
		{"1.0.2k", "1.0.3", -1, true},
		{"8.9p1", "8.9", 1, true},
		{"8.9p1", "8.9p2", -1, true},
		{"8.9p1", "8.10", -1, true},
		{"9.6p1", "9.8", -1, true},
		{"9.8p1", "9.8", 1, true},
		{"1.0rc1", "1.0", -1, true},
		{"1.0rc1", "1.0rc2", -1, true},
		{"1.0beta2", "1.0rc1", -1, true},
		{"1.0-alpha", "1.0", -1, true},
		{"1.0", "1.0-beta1", 1, true},
		{"1.0.2K", "1.0.2k", 0, true},
		{"1_2_3", "1.2.3", 0, true},
		{"99999999999999999999999.1", "99999999999999999999998.9", 1, true},
		// cannot be placed: a letter where the other has a number, or an unknown word
		{"1.0.2k", "1.0.2.1", 0, false},
		{"1.0-final", "1.0", 0, false},
		{"", "1.0", 0, false},
		{"1.0", "", 0, false},
		{"1.0..2", "1.0.2", 0, false},
	}
	for _, c := range cases {
		got, ok := compareVersions(c.a, c.b)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.wantOK)
		}
		if ok {
			back, backOK := compareVersions(c.b, c.a)
			if !backOK || back != -got {
				t.Errorf("compareVersions(%q, %q) = %d, %v; not the mirror of %d", c.b, c.a, back, backOK, got)
			}
		}
	}
}
