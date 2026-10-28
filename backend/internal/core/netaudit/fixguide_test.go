package netaudit

import (
	"net/netip"
	"regexp"
	"strings"
	"testing"
)

// Every check that has a one-line suggested fix has a guide, and the guide is complete: steps, a way back and the risk.
func TestGuides_EveryCheckWithASuggestedFixHasACompleteGuide(t *testing.T) {
	for check := range remediations {
		g, ok := GuideFor(check)
		if !ok {
			t.Errorf("%s has a suggested fix but no guide", check)
			continue
		}
		if len(g.Steps) == 0 || g.Undo == "" || g.Risk == "" {
			t.Errorf("%s: a guide needs steps, the risk and the way back: %+v", check, g)
		}
		for _, c := range g.Commands {
			if c.Platform == "" || strings.TrimSpace(c.Command) == "" {
				t.Errorf("%s: a command needs a platform and text: %+v", check, c)
			}
		}
	}
}

func TestGuides_OnlyChecksHaveGuides(t *testing.T) {
	for _, check := range []string{CheckOpenPort, "", "nothing_like_it"} {
		if _, ok := GuideFor(check); ok {
			t.Errorf("%q must have no guide", check)
		}
	}
}

// The guides are text this repository owns: no address of a real device, no leftover marker, nothing a model wrote.
func TestGuides_ContainNoAddressesOrPlaceholders(t *testing.T) {
	addr := regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	for check := range remediations {
		g, _ := GuideFor(check)
		text := strings.Join(append(append([]string{g.Undo, g.Risk}, g.Steps...), commandsText(g)...), "\n")
		for _, m := range addr.FindAllString(text, -1) {
			// 127.0.0.1 and ::1 are the loopback addresses a hardening step binds to.
			if a, err := netip.ParseAddr(m); err != nil || !a.IsLoopback() {
				t.Errorf("%s: guide contains the address %s", check, m)
			}
		}
		for _, bad := range []string{"TODO", "FIXME", "XXX", "lorem"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s: guide contains %q", check, bad)
			}
		}
	}
}

func commandsText(g FixGuide) []string {
	var out []string
	for _, c := range g.Commands {
		out = append(out, c.Command)
	}
	return out
}
