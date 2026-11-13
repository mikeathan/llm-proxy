package app

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/netaudit/hosttraffic"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"

	stacksdk "llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit/framewire"
	"llm-proxy/internal/core/netaudit/nmap"
	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/port"
	"llm-proxy/internal/platform/privhelper/session"
	"llm-proxy/internal/platform/scanscope"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// helperSocket is where the privileged helper's net-class unit listens (docs/services/llm-proxy-helper-net.socket). Nothing is there until
// the operator installs the unit, and then every call reports the helper unavailable.
const helperSocket = "/run/llm-proxy-helper-net.sock"

// helperRadioSocket is where the radio-class unit listens (docs/services/llm-proxy-helper-radio.socket, Constitution I.16). It has its own
// units, installed with the net class by `setup.sh --helper` (or alone); with no unit there, a radio action reports the helper unavailable.
const helperRadioSocket = "/run/llm-proxy-helper-radio.sock"

// helperGatesFile is the root-owned drop-in `scripts/install-helper.sh` writes: the actions and the scan ranges the helper holds. It is
// world-readable, so the service can show what the install did, and nothing the service can write decides what the helper does.
const helperGatesFile = "/etc/systemd/system/llm-proxy-helper-net@.service.d/10-gates.conf"

// helperRadioGatesFile is the same drop-in of the radio class, which has its own units (`install-helper.sh radio`, run by `setup.sh --helper`).
const helperRadioGatesFile = "/etc/systemd/system/llm-proxy-helper-radio@.service.d/10-gates.conf"

// helperRoutes is the service's side of the privileged helper (ADR-0034, Constitution I.11), built once here: the one-use grant store, the
// client that reads the sandbox's state on every call, and the port that checks a grant before the client is used. It is the only place
// that builds them, which an architecture test holds.
type helperRoutes struct {
	handlers map[string]http.HandlerFunc
	status   http.HandlerFunc
	// capture answers the last capture_window summary (GET /admin/api/netaudit/capture-window).
	capture http.HandlerFunc
	// traffic answers the last capture_host summary (GET /admin/api/netaudit/host-traffic).
	traffic http.HandlerFunc
	// sessions are the routes of bounded sessions (Constitution I.17): list, begin (a step-up route), end one, end all.
	sessionList, sessionBegin, sessionEnd, sessionEndAll http.HandlerFunc
	// port and sessions let a stack be given the port that serves its sessions (StackPrivileged).
	port     *port.Port
	sessions *session.Store
	on       func(action string) bool
}

// sessionStacks are the stacks that are given a port which serves bounded sessions. A session names one of them.
var sessionStacks = []string{"radio"}

// StackPrivileged is the Privileged port of one stack: a request with a grant takes the grant path, a request with none is served only
// while an open session of that stack approved exactly that request (Constitution I.17). The stack never holds a token.
func (h *helperRoutes) StackPrivileged(stack string) stacksdk.Privileged {
	return h.port.ForStack(stack, h.sessions, h.on)
}

// switchedOffSessions ends sessions whose action has been switched off, so a session never shows as open while nothing can use it. The port
// checks the switch on every use as well; this only keeps the list honest.
type switchedOffSessions struct {
	*session.Store
	on func(action string) bool
}

func (s switchedOffSessions) Open() (open, ended []session.View) {
	for _, a := range catalog.All() {
		if a.Standing && !s.on(a.ID) {
			s.EndAction(a.ID)
		}
	}
	return s.Store.Open()
}

// helperSinks is where the helper's results are kept: scan results on the device inventory, and the last summary of each look. Any may be
// nil (a test, or a Network module that is not wired): a result with no place to go is shown and not kept.
type helperSinks struct {
	Probes   probeSink
	Captures captureSink
	Traffic  hostTrafficSink
	// Owners gives, at the moment of each look, the way to say who holds an internet address, for the traffic view: nil until the operator has
	// downloaded the table, and whenever the table is empty.
	Owners func() hosttraffic.Owners
	// Self says which packets of a traffic look are this computer's (see trafficSelf).
	Self func() hosttraffic.Self
}

// liveSandbox reads the sandbox provider at the moment of each call: the provider is built after the routes are, and the interlock must see
// what is enforced now, not what was enforced at start-up.
type liveSandbox struct{ ctx *AppContext }

func (l liveSandbox) Effective() isolation.EffectiveState {
	if l.ctx.sandboxProv == nil {
		return isolation.EffectiveState{}
	}
	return l.ctx.sandboxProv.Effective()
}

// newHelperRoutes builds the helper's routes. policy returns the scan policy the operator declared in the Network settings (nil when the
// Network module is not wired, as in a test): every helper call is held to it here, because the helper holds no range of its own.
func newHelperRoutes(ctx *AppContext, policy func() scanscope.Policy, sinks helperSinks) *helperRoutes {
	if policy == nil {
		policy = func() scanscope.Policy { return scanscope.Policy{} }
	}
	grants := grant.NewStore(grant.Options{})
	helperClient := client.New(helperSocket, liveSandbox{ctx}).WithClass(catalog.ClassRadio, helperRadioSocket)
	helper := port.New(port.Options{Grants: grants, Helper: helperClient, Ranges: declaredRanges{policy}})
	on := func(action string) bool { return ctx.HostSettings().Sandboxing.HelperActionOn(action) }
	sessions := session.New(session.Options{Log: logging.Info})
	sessionDeps := api.HelperSessionDeps{Sessions: switchedOffSessions{sessions, on}, On: on, Stacks: sessionStacks}
	keep := keepHelperResult(sinks)
	running := api.NewRunningActions()
	h := &helperRoutes{handlers: map[string]http.HandlerFunc{}, port: helper, sessions: sessions, on: on}
	h.sessionList, h.sessionBegin = api.HelperSessionsList(sessionDeps), api.HelperSessionsBegin(sessionDeps)
	h.sessionEnd, h.sessionEndAll = api.HelperSessionEnd(sessionDeps), api.HelperSessionsEndAll(sessionDeps)
	for _, rt := range api.HelperRoutes() {
		id := rt.Path[len("/admin/api/helper/"):]
		h.handlers[rt.Path] = api.HelperHandler(id, api.HelperDeps{Grants: grants, Run: helper, On: on, Parse: helperParser{owners: sinks.Owners, self: sinks.Self}.parse, Keep: keep, Defaults: helperDefaults(defaultInterface), OmitRaw: keepsToItself, Running: running})
	}
	h.capture = captureWindowHandler(sinks.Captures)
	h.traffic = hostTrafficHandler(sinks.Traffic)
	h.status = api.HelperStatusHandler(api.HelperSource{
		Supported: func() bool { return runtime.GOOS == "linux" },
		Installed: helperSocketPresent,
		Interlock: helperClient.Ready,
		Gates:     func() ([]string, bool) { return readAllHelperActions(helperGatesFile, helperRadioGatesFile) },
		Declared:  func() []string { return rangeStrings(policy()) },
		On:        on,
		Running:   running.Running,
	})
	return h
}

// keepsToItself says which actions' raw output stays in the service: a capture's bytes are frames of other devices' traffic (a radio window's too: Constitution I.16), so the answer to the
// browser is the parsed summary and never the bytes (Constitution I.13). A scan's raw output is still sent, so the operator can read what came
// back when the parser could not.
func keepsToItself(action string) bool {
	return action == "capture_window" || action == "capture_host" || action == "radio_listen"
}

// helperParser reads each action's raw output. owners, when set, gives the way to name the holders of the internet addresses a traffic look saw.
type helperParser struct {
	owners func() hosttraffic.Owners
	self   func() hosttraffic.Self
}

// currentOwners is the table of address owners as it is now, or nil when there is none.
func (p helperParser) currentOwners() hosttraffic.Owners {
	if p.owners == nil {
		return nil
	}
	return p.owners()
}

// parse reads one action's raw output in this unprivileged process (Constitution I.11, I.13): a scan is nmap's XML and a capture is
// a few kept bytes of each frame, and each parser keeps only typed, cleaned and bounded fields. An action added to the catalogue gets its case here; one
// without a case is answered as raw bytes only, which the route says in parse_error.
func (p helperParser) parse(action string, data []byte) (any, error) {
	switch action {
	case "scan_udp", "os_detect":
		return nmap.ParseProbe(data)
	case "capture_window":
		return framewire.Summarize(data)
	case "radio_listen":
		return summarizeRadioWindow(data)
	case "capture_host":
		var self hosttraffic.Self
		if p.self != nil {
			self = p.self()
		}
		return readHostTraffic(data, self, p.currentOwners())
	}
	return nil, fmt.Errorf("no reader for the output of %q", action)
}

// probeSink is where a scan's result is kept: the device inventory, which holds the last result on the device at the target's address.
type probeSink interface {
	RecordUDPScan(ip string, ports []models.ProbeUDPPort) error
	RecordOSGuess(ip string, guesses []models.ProbeOSGuess) error
}

// helperTarget is the target of a helper request, in the form the inventory keys devices by.
type helperTarget struct {
	Target string `json:"target"`
}

// keepHelperResult keeps a parsed scan on the device at its target. A result that cannot be kept (a body or result of an unexpected shape, a
// target no device holds) is logged and dropped: the operator already has the answer on screen, and a failed save must not fail the scan.
func keepHelperResult(sinks helperSinks) api.HelperKeeper {
	return func(action string, params []byte, parsed any) {
		if sum, ok := parsed.(models.CaptureSummary); ok {
			keepCapture(sinks.Captures, linkPrefixes, params, sum)
			return
		}
		if traffic, ok := parsed.(models.HostTraffic); ok {
			keepHostTraffic(sinks.Traffic, params, traffic)
			return
		}
		sink := sinks.Probes
		if sink == nil {
			return
		}
		var req helperTarget
		probe, isProbe := parsed.(models.DeviceProbe)
		if json.Unmarshal(params, &req) != nil || !isProbe {
			return
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(req.Target))
		if err != nil {
			return
		}
		switch action {
		case "scan_udp":
			err = sink.RecordUDPScan(addr.String(), probe.UDPPorts)
		case "os_detect":
			err = sink.RecordOSGuess(addr.String(), probe.OS)
		default:
			return
		}
		if err != nil {
			logging.Warn("a helper scan result was not kept", "action", action, "error", err)
		}
	}
}

// helperSocketPresent reports whether the helper's socket exists: the unit is installed and its socket is listening.
func helperSocketPresent() bool {
	st, err := os.Stat(helperSocket)
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// readHelperActions reads the install's drop-in: the line `Environment=HELPER_ACTIONS=a,b`. ok is false when the file cannot be read or
// holds no such line.
func readHelperActions(path string) (actions []string, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, found := strings.CutPrefix(strings.TrimSpace(line), "Environment=HELPER_ACTIONS="); found {
			return splitCSV(v), true
		}
	}
	return nil, false
}

// readAllHelperActions joins the actions of every class's drop-in that can be read. ok is false only when none can, so a class that is
// not installed shows its actions as missing while the installed ones show as present.
func readAllHelperActions(paths ...string) (actions []string, ok bool) {
	for _, p := range paths {
		if a, found := readHelperActions(p); found {
			actions, ok = append(actions, a...), true
		}
	}
	return actions, ok
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// declaredRanges holds a helper call's target to the scan ranges the operator declared in the Network settings, read each time so a change
// applies at once. It answers port.Ranges.
type declaredRanges struct{ policy func() scanscope.Policy }

func (d declaredRanges) Check(target string) error {
	p := d.policy()
	if len(p.Allowed) == 0 {
		return port.ErrNoRangeDeclared
	}
	addr, err := netip.ParseAddr(target)
	if err != nil {
		return fmt.Errorf("target %q: %w", target, err)
	}
	if err := p.Check(addr); err != nil {
		return fmt.Errorf("%w: %v", port.ErrOutsideRange, err)
	}
	return nil
}

// rangeStrings lists a policy's ranges as CIDR blocks, for the status the Network page shows.
func rangeStrings(p scanscope.Policy) []string {
	out := make([]string, 0, len(p.Allowed))
	for _, r := range p.Allowed {
		out = append(out, r.String())
	}
	return out
}

// scopePolicy reads the scan policy of the Network settings; none when the Network module is not wired.
func scopePolicy(s *AppServices) func() scanscope.Policy {
	return func() scanscope.Policy {
		if s.netaudit == nil || s.netaudit.Scope == nil {
			return scanscope.Policy{}
		}
		return s.netaudit.Scope.Policy()
	}
}

// validateHelperActions refuses a settings write that turns on an action the catalogue does not hold: a typo must not look like a switch
// that is on.
func validateHelperActions(s models.HostSettings) error {
	for _, id := range s.Sandboxing.HelperActions {
		if _, err := catalog.Lookup(id); err != nil {
			return fmt.Errorf("helper_actions names %q, which is not an action of the privileged helper", id)
		}
	}
	return nil
}
