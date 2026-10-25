package netaudit

import (
	"slices"
	"testing"
)

func TestMode(t *testing.T) {
	if !slices.Equal(Modes(), []Mode{ModeDiscover, ModePorts, ModeServices}) {
		t.Errorf("Modes() = %v", Modes())
	}
	if Mode("vuln").Valid() || Mode("").Valid() {
		t.Error("an unknown mode must not be valid")
	}
	if !ModeServices.Spec().NeedsNmap || ModePorts.Spec().NeedsNmap {
		t.Error("only service detection needs nmap")
	}
	for _, m := range Modes() {
		if m.Spec().Intrusive {
			t.Errorf("%s: no intrusive mode exists yet, so none may be marked as one", m)
		}
	}
}
