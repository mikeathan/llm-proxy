package neighbors

import (
	"slices"
	"strings"
	"testing"
)

func TestDiscoverable_KeepsPlainInterfaceNamesOnceAndBoundsThem(t *testing.T) {
	got := discoverable([]string{"wlp2s0", "wlp2s0", "", "eth0.100", "br-0123456789ab", "bad name", "x;rm", strings.Repeat("a", 16), "eth1"})
	want := []string{"wlp2s0", "eth0.100", "br-0123456789ab", "eth1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	many := make([]string, 0, 20)
	for i := range 20 {
		many = append(many, "eth"+string(rune('a'+i)))
	}
	if got := discoverable(many); len(got) != maxDiscover6Interfaces {
		t.Errorf("at most %d interfaces, got %d", maxDiscover6Interfaces, len(got))
	}
}
