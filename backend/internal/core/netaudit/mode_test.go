package netaudit

import (
	"slices"
	"testing"
)

func TestMode(t *testing.T) {
	if !slices.Equal(Modes(), []Mode{ModeChecks, ModeDiscover, ModePorts, ModeServices}) {
		t.Errorf("Modes() = %v", Modes())
	}
	if Mode("vuln").Valid() || Mode("").Valid() {
		t.Error("an unknown mode must not be valid")
	}
	if !ModeServices.Spec().NeedsNmap || !ModeChecks.Spec().NeedsNmap || ModePorts.Spec().NeedsNmap {
		t.Error("service detection and checks need nmap; a plain port scan does not")
	}
	// The modes an ordinary scan (a person's request, an agent's tool call, a nightly audit) may use never include an
	// intrusive one; deep is valid, but only the operator's own endpoint may start it.
	for _, m := range Modes() {
		if m.Spec().Intrusive {
			t.Errorf("%s is intrusive and must not be offered to an ordinary scan", m)
		}
	}
	if !ModeDeep.Valid() || !ModeDeep.Spec().Intrusive || !ModeDeep.Spec().NeedsNmap {
		t.Errorf("deep must be a valid, intrusive mode that needs nmap: %+v", ModeDeep.Spec())
	}
	if slices.Contains(Modes(), ModeDeep) {
		t.Error("Modes() lists what an ordinary scan accepts; deep is not one of them")
	}
}
