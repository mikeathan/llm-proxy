package models

import "testing"

// A new install starts its model server on this computer only: the proxy reaches it there, and an open model port is not
// behind the /v1 key (Constitution I.5: a new install listens on this machine only). An existing settings file keeps its value.
func TestDefaultAppConfig_ModelServerListensOnThisComputerOnly(t *testing.T) {
	if got := DefaultAppConfig().Server.ModelHost; got != AddrLocalhost {
		t.Errorf("default model host = %q, want %q", got, AddrLocalhost)
	}
}
