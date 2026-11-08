package app

import (
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/port"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/internal/platform/scanscope"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// helperSocket is where the privileged helper's net-class unit listens (docs/services/llm-proxy-helper-net.socket). Nothing is there until
// the operator installs the unit, and then every call reports the helper unavailable.
const helperSocket = "/run/llm-proxy-helper-net.sock"

// helperGatesFile is the root-owned drop-in `scripts/install-helper.sh` writes: the actions and the scan ranges the helper holds. It is
// world-readable, so the service can show what the install did, and nothing the service can write decides what the helper does.
const helperGatesFile = "/etc/systemd/system/llm-proxy-helper-net@.service.d/10-gates.conf"

// helperRoutes is the service's side of the privileged helper (ADR-0034, Constitution I.11), built once here: the one-use grant store, the
// client that reads the sandbox's state on every call, and the port that checks a grant before the client is used. It is the only place
// that builds them, which an architecture test holds.
type helperRoutes struct {
	handlers map[string]http.HandlerFunc
	status   http.HandlerFunc
}

// liveSandbox reads the sandbox provider at the moment of each call: the provider is built after the routes are, and the interlock must see
// what is enforced now, not what was enforced at start-up.
type liveSandbox struct{ ctx *AppContext }

func (l liveSandbox) Effective() sandbox.EffectiveState {
	if l.ctx.sandboxProv == nil {
		return sandbox.EffectiveState{}
	}
	return l.ctx.sandboxProv.Effective()
}

// newHelperRoutes builds the helper's routes. policy returns the scan policy the operator declared in the Network settings (nil when the
// Network module is not wired, as in a test): every helper call is held to it here, because the helper holds no range of its own.
func newHelperRoutes(ctx *AppContext, policy func() scanscope.Policy) *helperRoutes {
	if policy == nil {
		policy = func() scanscope.Policy { return scanscope.Policy{} }
	}
	grants := grant.NewStore(grant.Options{})
	helperClient := client.New(helperSocket, liveSandbox{ctx})
	helper := port.New(port.Options{Grants: grants, Helper: helperClient, Ranges: declaredRanges{policy}})
	on := func(action string) bool { return ctx.HostSettings().Sandboxing.HelperActionOn(action) }
	h := &helperRoutes{handlers: map[string]http.HandlerFunc{}}
	for _, rt := range api.HelperRoutes() {
		id := rt.Path[len("/admin/api/helper/"):]
		h.handlers[rt.Path] = api.HelperHandler(id, grants, helper, on)
	}
	h.status = api.HelperStatusHandler(api.HelperSource{
		Supported: func() bool { return runtime.GOOS == "linux" },
		Installed: helperSocketPresent,
		Interlock: helperClient.Ready,
		Gates:     func() ([]string, bool) { return readHelperActions(helperGatesFile) },
		Declared:  func() []string { return rangeStrings(policy()) },
		On:        on,
	})
	return h
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
