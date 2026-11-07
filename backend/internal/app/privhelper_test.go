package app

import (
	"testing"

	"llm-proxy/internal/platform/privhelper/client"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

func settingsWith(ids ...string) models.HostSettings {
	return models.HostSettings{Sandboxing: models.HostSandboxingConfig{HelperActions: ids}}
}

func TestAHelperActionMustBeInTheCatalogueToBeTurnedOn(t *testing.T) {
	for _, ok := range [][]string{nil, {}, {"scan_udp"}, {"scan_udp", "os_detect"}} {
		if err := validateHelperActions(settingsWith(ok...)); err != nil {
			t.Errorf("%v refused: %v", ok, err)
		}
	}
	for _, bad := range [][]string{{"scan_upd"}, {"scan_udp", "rm_rf"}, {""}} {
		if err := validateHelperActions(settingsWith(bad...)); err == nil {
			t.Errorf("%v must be refused: a typo must not look like a switch that is on", bad)
		}
	}
}

func TestEveryHelperRouteHasAHandler(t *testing.T) {
	h := newHelperRoutes(&AppContext{})
	routes := api.HelperRoutes()
	if len(routes) == 0 || len(h.handlers) != len(routes) {
		t.Fatalf("%d handlers for %d routes", len(h.handlers), len(routes))
	}
	for _, rt := range routes {
		if h.handlers[rt.Path] == nil {
			t.Errorf("no handler for %s", rt.Path)
		}
	}
}

// With no sandbox provider the interlock must read "nothing enforced", so the client refuses instead of calling the helper.
func TestWithoutASandboxTheInterlockReportsNothingEnforced(t *testing.T) {
	e := liveSandbox{&AppContext{}}.Effective()
	if _, unsafe := client.New("/nonexistent", liveSandbox{&AppContext{}}).Call(t.Context(), "scan_udp",
		[]byte(`{"target":"192.168.1.5","ports":[53]}`)); unsafe == nil || e.Filesystem.Mechanism != "" && e.Filesystem.Mechanism != "none" {
		t.Errorf("effective %+v, error %v", e, unsafe)
	}
}

func TestOnlyTheSettingTheOperatorWroteTurnsAnActionOn(t *testing.T) {
	c := models.HostSandboxingConfig{HelperActions: []string{"scan_udp"}}
	if !c.HelperActionOn("scan_udp") || c.HelperActionOn("os_detect") || (models.HostSandboxingConfig{}).HelperActionOn("scan_udp") {
		t.Error("an action is on only when it is in the list; absent is off")
	}
}
