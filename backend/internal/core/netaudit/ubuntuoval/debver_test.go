package ubuntuoval

import (
	"math/rand"
	"testing"
)

func TestCompare_FollowsDpkgsOrderingOnKnownPairs(t *testing.T) {
	less := [][2]string{
		{"1.0~rc1", "1.0"},
		{"1.0~rc1", "1.0~rc2"},
		{"1.0", "1.0+b1"},
		{"1.0", "1.0a"},
		{"1.9", "1.10"},
		{"1.0", "1.0-1"},
		{"1:9.9", "2:1.0"},
		{"9.9", "1:0.1"},
		{"3.0.13-0ubuntu3", "3.0.13-0ubuntu3.1"},
		{"3.0.13-0ubuntu3.1", "3.0.13-0ubuntu3.2"},
		{"1.0-1ubuntu1~esm1", "1.0-1ubuntu1"},
		{"1.2.3-0ubuntu0.24.04.1", "1.2.3-0ubuntu0.24.04.2"},
		{"2.39-0ubuntu8.3", "2.39-0ubuntu8.10"},
		{"4:24.2.3-0ubuntu0.24.04.1", "4:24.2.3-0ubuntu0.24.04.2"},
		{"1.0-1", "1.0-1+deb12u1"},
		{"1.0", "1.0.1"},
		{"a", "b"},
		{"1.0-01", "1.0-2"},
	}
	for _, p := range less {
		if Compare(p[0], p[1]) >= 0 {
			t.Errorf("%q should be older than %q (got %d)", p[0], p[1], Compare(p[0], p[1]))
		}
		if Compare(p[1], p[0]) <= 0 {
			t.Errorf("%q should be newer than %q (got %d)", p[1], p[0], Compare(p[1], p[0]))
		}
	}
}

func TestCompare_EqualVersionsAreEqualWhateverTheirSpelling(t *testing.T) {
	for _, p := range [][2]string{{"1.0", "1.0"}, {"0:1.0", "1.0"}, {"1.0-0", "1.0-0"}, {"1.00", "1.0"}, {"1.0-1", "1.0-1"}, {"", ""}} {
		if got := Compare(p[0], p[1]); got != 0 {
			t.Errorf("Compare(%q, %q) = %d, want 0", p[0], p[1], got)
		}
	}
}

func TestCompare_IsAnOrderAndNeverPanicsOnJunk(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const alphabet = "0123456789.+~-:abcXYZ "
	gen := func() string {
		b := make([]byte, rng.Intn(14))
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(b)
	}
	for i := 0; i < 20000; i++ {
		a, b, c := gen(), gen(), gen()
		ab, ba := Compare(a, b), Compare(b, a)
		if (ab < 0) != (ba > 0) || (ab == 0) != (ba == 0) {
			t.Fatalf("not antisymmetric: %q %q -> %d, %d", a, b, ab, ba)
		}
		if Compare(a, b) < 0 && Compare(b, c) < 0 && Compare(a, c) >= 0 {
			t.Fatalf("not transitive: %q < %q < %q but %d", a, b, c, Compare(a, c))
		}
	}
}
