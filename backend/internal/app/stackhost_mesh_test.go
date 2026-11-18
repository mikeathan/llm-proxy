package app

import (
	"slices"
	"testing"

	"llm-proxy-stacks/sdk"
)

// Asking for the mesh router's list of joined devices may cause a read of the router, so only the Radio stack, which asks while a watch runs,
// is given the port (Constitution I.15, ADR-0059).
func TestOnlyTheRadioStackIsGivenTheRoutersListOfDevices(t *testing.T) {
	for _, m := range []stacksdk.Manifest{networkStack().Manifest(), agentsStack().Manifest(), radioStack().Manifest()} {
		if got, want := slices.Contains(m.Requires, stacksdk.PortMeshClients), m.ID == radioStack().Manifest().ID; got != want {
			t.Errorf("stack %q requires the meshclients port: %v", m.ID, got)
		}
	}
}
