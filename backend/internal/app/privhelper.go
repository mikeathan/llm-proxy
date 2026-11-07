package app

import (
	"fmt"
	"net/http"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/client"
	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/port"
	"llm-proxy/internal/platform/sandbox"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// helperSocket is where the privileged helper's net-class unit listens (docs/services/llm-proxy-helper-net.socket). Nothing is there until
// the operator installs the unit, and then every call reports the helper unavailable.
const helperSocket = "/run/llm-proxy-helper-net.sock"

// helperRoutes is the service's side of the privileged helper (ADR-0034, Constitution I.11), built once here: the one-use grant store, the
// client that reads the sandbox's state on every call, and the port that checks a grant before the client is used. It is the only place
// that builds them, which an architecture test holds.
type helperRoutes struct {
	handlers map[string]http.HandlerFunc
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

func newHelperRoutes(ctx *AppContext) *helperRoutes {
	grants := grant.NewStore(grant.Options{})
	helper := port.New(grants, client.New(helperSocket, liveSandbox{ctx}))
	on := func(action string) bool { return ctx.HostSettings().Sandboxing.HelperActionOn(action) }
	h := &helperRoutes{handlers: map[string]http.HandlerFunc{}}
	for _, rt := range api.HelperRoutes() {
		id := rt.Path[len("/admin/api/helper/"):]
		h.handlers[rt.Path] = api.HelperHandler(id, grants, helper, on)
	}
	return h
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
