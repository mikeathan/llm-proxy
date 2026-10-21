package tools

import (
	"strings"
	"testing"
)

// fitHeadTail keeps the start and the end of a text, with a marker between, so that the result's JSON encoding is within
// the cap. proxy.TruncateResult sizes by raw characters and always adds its marker, so its output was over the cap even
// before escaping (every quote becomes two characters): a result cut by it was cut a second time by the agent.
func TestFitHeadTail(t *testing.T) {
	plain := strings.Repeat("0123456789", 800)
	escapeHeavy := strings.Repeat(`say "hi" & <go>\n`, 700)
	for name, text := range map[string]string{"plain": plain, "escape heavy": escapeHeavy} {
		for _, capChars := range []int{3000, 5000, 12000} {
			out := fitHeadTail(text, capChars)
			if encodedLen(out) > capChars {
				t.Errorf("%s cap %d: encoded %d, over the cap", name, capChars, encodedLen(out))
			}
			if !strings.HasPrefix(out, text[:50]) || !strings.HasSuffix(out, text[len(text)-50:]) {
				t.Errorf("%s cap %d: the start and the end of the text must both be kept", name, capChars)
			}
		}
	}
	if got := fitHeadTail("short", 3000); got != "short" {
		t.Errorf("a text that fits must be returned as it is, got %q", got)
	}
}
