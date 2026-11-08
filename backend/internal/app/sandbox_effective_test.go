package app

import (
	"testing"

	"llm-proxy/internal/platform/isolation"
)

// What the host-settings page reads must carry every surface the sandbox reports, the Unix-socket denial included, or a "permission
// denied" in an agent shell would have no visible cause.
func TestSandboxEffective_CarriesEverySurfaceWithItsReason(t *testing.T) {
	got := sandboxEffective(isolation.EffectiveState{
		Filesystem:  isolation.SurfaceState{Mechanism: isolation.EnforcementLandlock},
		Network:     isolation.SurfaceState{Mechanism: isolation.EnforcementNone, Reason: "gap"},
		UnixSockets: isolation.SurfaceState{Mechanism: isolation.EnforcementSeccomp, Reason: "refused for children"},
		Provider:    "landlock",
	})
	if got.Filesystem.Mechanism != "landlock" || got.Network.Reason != "gap" || got.Provider != "landlock" {
		t.Errorf("projection = %+v", got)
	}
	if got.UnixSockets.Mechanism != "seccomp" || got.UnixSockets.Reason != "refused for children" {
		t.Errorf("unix sockets = %+v", got.UnixSockets)
	}
}
