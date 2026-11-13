package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/port"
	"llm-proxy/internal/platform/privhelper/session"
	"llm-proxy/internal/platform/scanscope"
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
	h := newHelperRoutes(&AppContext{}, nil, helperSinks{})
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

func TestReadHelperActionsReadsWhatTheInstallWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "10-gates.conf")
	if err := os.WriteFile(path, []byte("[Service]\nEnvironment=HELPER_ACTIONS=scan_udp, os_detect\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if actions, ok := readHelperActions(path); !ok || !slices.Equal(actions, []string{"scan_udp", "os_detect"}) {
		t.Errorf("%v %v", actions, ok)
	}
}

func TestReadHelperActionsSaysUnknownWhenItCannotRead(t *testing.T) {
	if _, ok := readHelperActions(filepath.Join(t.TempDir(), "absent.conf")); ok {
		t.Error("a missing file must read as unknown")
	}
	path := filepath.Join(t.TempDir(), "other.conf")
	_ = os.WriteFile(path, []byte("[Service]\nEnvironment=SOMETHING=else\n"), 0o644)
	if _, ok := readHelperActions(path); ok {
		t.Error("a file without the line must read as unknown")
	}
}

// The radio class is a separate install with its own drop-in, so the status must read the actions of every class's file, or a radio action
// that is installed shows as missing.
func TestReadAllHelperActionsJoinsTheFilesOfEveryInstalledClass(t *testing.T) {
	dir := t.TempDir()
	write := func(name, actions string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("[Service]\nEnvironment=HELPER_ACTIONS="+actions+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	net, radio := write("net.conf", "scan_udp"), write("radio.conf", "radio_probe,radio_listen")
	absent := filepath.Join(dir, "absent.conf")
	if got, ok := readAllHelperActions(net, radio); !ok || !slices.Equal(got, []string{"scan_udp", "radio_probe", "radio_listen"}) {
		t.Errorf("both installed: %v %v", got, ok)
	}
	if got, ok := readAllHelperActions(net, absent); !ok || !slices.Equal(got, []string{"scan_udp"}) {
		t.Errorf("only net installed: %v %v", got, ok)
	}
	if _, ok := readAllHelperActions(absent, absent); ok {
		t.Error("no readable file must read as unknown")
	}
}

// The helper holds no range of its own, so these are the checks that keep a helper call inside what the operator declared.
func TestAHelperTargetMustBeInsideTheDeclaredRanges(t *testing.T) {
	policy := func(cidrs ...string) func() scanscope.Policy {
		return func() scanscope.Policy { return scanscope.PolicyFromCIDRs(cidrs) }
	}
	cases := []struct {
		name   string
		ranges []string
		target string
		want   error
	}{
		{"inside", []string{"192.168.50.0/24"}, "192.168.50.10", nil},
		{"private but not declared", []string{"192.168.50.0/24"}, "192.168.51.10", port.ErrOutsideRange},
		{"another private block", []string{"192.168.50.0/24"}, "10.0.0.5", port.ErrOutsideRange},
		{"nothing declared", nil, "192.168.50.10", port.ErrNoRangeDeclared},
		{"a public address is never in scope", []string{"8.8.8.0/24"}, "8.8.8.8", port.ErrOutsideRange},
	}
	for _, c := range cases {
		if err := (declaredRanges{policy(c.ranges...)}).Check(c.target); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	if err := (declaredRanges{policy("192.168.50.0/24")}).Check("not-an-address"); err == nil || errors.Is(err, port.ErrOutsideRange) {
		t.Errorf("an unparsable target is an error, not a range answer: %v", err)
	}
}

func TestTheDeclaredRangesAreReadFromTheNetworkSettingsEachTime(t *testing.T) {
	if got := scopePolicy(&AppServices{})(); len(got.Allowed) != 0 {
		t.Errorf("with no Network module there are no ranges: %v", got.Allowed)
	}
	if got := rangeStrings(scanscope.PolicyFromCIDRs([]string{"192.168.50.0/24", "10.0.0.0/8"})); !slices.Equal(got, []string{"192.168.50.0/24", "10.0.0.0/8"}) {
		t.Errorf("%v", got)
	}
}

type recordingSink struct {
	udp map[string][]models.ProbeUDPPort
	os  map[string][]models.ProbeOSGuess
	err error
}

func (r *recordingSink) RecordUDPScan(ip string, p []models.ProbeUDPPort) error {
	if r.udp == nil {
		r.udp = map[string][]models.ProbeUDPPort{}
	}
	r.udp[ip] = p
	return r.err
}

func (r *recordingSink) RecordOSGuess(ip string, g []models.ProbeOSGuess) error {
	if r.os == nil {
		r.os = map[string][]models.ProbeOSGuess{}
	}
	r.os[ip] = g
	return r.err
}

func TestAParsedScanIsKeptOnTheDeviceAtItsTarget(t *testing.T) {
	sink := &recordingSink{}
	keep := keepHelperResult(helperSinks{Probes: sink})
	probe := models.DeviceProbe{UDPPorts: []models.ProbeUDPPort{{Port: 53, State: "open"}}, OS: []models.ProbeOSGuess{{Name: "Linux", Accuracy: 90}}}
	keep("scan_udp", []byte(`{"target":" 192.168.50.10 ","ports":[53]}`), probe)
	keep("os_detect", []byte(`{"target":"192.168.50.10"}`), probe)
	if len(sink.udp["192.168.50.10"]) != 1 || len(sink.os["192.168.50.10"]) != 1 {
		t.Errorf("kept udp %v os %v (the target is written in the form the inventory keys by)", sink.udp, sink.os)
	}
}

func TestAResultThatCannotBeKeptIsDroppedWithoutPanicking(t *testing.T) {
	sink := &recordingSink{err: errors.New("no such device")}
	keep := keepHelperResult(helperSinks{Probes: sink})
	probe := models.DeviceProbe{}
	keep("scan_udp", []byte(`not json`), probe)
	keep("scan_udp", []byte(`{"target":"nope"}`), probe)
	keep("scan_udp", []byte(`{"target":"192.168.50.10"}`), "not a probe")
	keep("unknown_action", []byte(`{"target":"192.168.50.10"}`), probe)
	keep("scan_udp", []byte(`{"target":"192.168.50.10"}`), probe) // the sink errors: logged, not raised
	if len(sink.udp) != 1 || len(sink.os) != 0 {
		t.Errorf("udp %v os %v: only the one well-formed call reaches the sink", sink.udp, sink.os)
	}
}

// The bytes of a radio window are other people's frames: the browser gets a summary or nothing, never the bytes (Constitution I.16). A probe's
// output is the system's own description of an adapter and may be shown.
func TestARadioWindowsRawBytesStayInTheService(t *testing.T) {
	if !keepsToItself("radio_listen") || keepsToItself("radio_probe") {
		t.Error("radio_listen's bytes stay in the service; radio_probe's do not")
	}
}

// A session never shows as open while its action is off: the list ends it, and the port refuses it on every use as well.
func TestASessionEndsWhenItsActionIsSwitchedOff(t *testing.T) {
	store := session.New(session.Options{})
	if _, err := store.Begin(session.Spec{Stack: "radio", Label: "x", TTL: time.Hour, MaxUses: 5,
		Requests: []session.Request{{Action: "radio_listen", Canonical: []byte(`{"iface":"w","channel":1,"seconds":20}`)}}}); err != nil {
		t.Fatal(err)
	}
	on := true
	view := switchedOffSessions{store, func(string) bool { return on }}
	if open, _ := view.Open(); len(open) != 1 {
		t.Fatalf("switched on: %d open", len(open))
	}
	on = false
	if open, ended := view.Open(); len(open) != 0 || len(ended) != 1 || ended[0].Ended != "the action was switched off" {
		t.Errorf("switched off: open %v ended %+v", open, ended)
	}
}

func TestAStackIsGivenThePortThatServesItsSessions(t *testing.T) {
	h := newHelperRoutes(&AppContext{}, nil, helperSinks{})
	if h.StackPrivileged("radio") == nil || h.sessionBegin == nil || h.sessionList == nil || h.sessionEnd == nil || h.sessionEndAll == nil {
		t.Error("the session routes and the stack port are built with the helper's routes")
	}
}
