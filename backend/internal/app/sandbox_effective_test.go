package app

import (
	"testing"

	"llm-proxy/internal/platform/sandbox"
)

// What the host-settings page reads must carry every surface the sandbox reports, the Unix-socket denial included, or a "permission
// denied" in an agent shell would have no visible cause.
func TestSandboxEffective_CarriesEverySurfaceWithItsReason(t *testing.T) {
	got := sandboxEffective(sandbox.EffectiveState{
		Filesystem:  sandbox.SurfaceState{Mechanism: sandbox.EnforcementLandlock},
		Network:     sandbox.SurfaceState{Mechanism: sandbox.EnforcementNone, Reason: "gap"},
		UnixSockets: sandbox.SurfaceState{Mechanism: sandbox.EnforcementSeccomp, Reason: "refused for children"},
		Provider:    "landlock",
	})
	if got.Filesystem.Mechanism != "landlock" || got.Network.Reason != "gap" || got.Provider != "landlock" {
		t.Errorf("projection = %+v", got)
	}
	if got.UnixSockets.Mechanism != "seccomp" || got.UnixSockets.Reason != "refused for children" {
		t.Errorf("unix sockets = %+v", got.UnixSockets)
	}
}
