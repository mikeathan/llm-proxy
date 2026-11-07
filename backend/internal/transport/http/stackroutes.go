package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/catalog"
)

// A stack declares its HTTP routes as data in its manifest (stacksdk.RouteSpec) and answers each with a plain-data Request and Response. The
// core mounts the routes here, under the same method and path the stack declares, so the frontend and the JSON are unchanged. Every route is
// inside the admin API, so Authenticate signs the caller in before the stack is called; the stack cannot lower that, and the only way to
// raise it is to declare AccessStepUp, which AuthenticateWith enforces with the core's own step-up check (plan P3b D5).

// GrantMinter mints the one-use grant for a privileged action (ADR-0034). It is an interface so that this package never imports the grant
// store: only the composition root builds one.
type GrantMinter interface {
	Mint(action string, canonical []byte) (string, error)
}

// MountStackRoutes registers each route the stack declares on mux. It refuses, before registering anything, a route the core could not keep
// safe: one outside /admin/api/, one under the sign-in routes (which need no credential), a step-up read (the step-up check skips reads, so
// it would protect nothing), a step-up route with a wildcard (the step-up table matches exact paths), one that names an action the
// catalogue does not hold (or any action when there is no minter), and one already registered.
func MountStackRoutes(mux *http.ServeMux, s stacksdk.Stack, minter GrantMinter) (err error) {
	routes := s.Manifest().Routes
	for _, rt := range routes {
		if err := checkStackRoute(rt, minter != nil); err != nil {
			return err
		}
	}
	defer func() {
		// ServeMux panics on a pattern it already holds; the composition root wants an error.
		if r := recover(); r != nil {
			err = fmt.Errorf("a stack route conflicts with a registered route: %v", r)
		}
	}()
	for _, rt := range routes {
		mux.HandleFunc(rt.Method+" "+rt.Path, stackHandler(s, rt, minter))
	}
	return nil
}

func checkStackRoute(rt stacksdk.RouteSpec, canMint bool) error {
	name := rt.Method + " " + rt.Path
	switch {
	case !strings.HasPrefix(rt.Path, "/admin/api/"):
		return fmt.Errorf("stack route %s: a stack route lives under /admin/api/", name)
	case strings.HasPrefix(rt.Path, authAPIPrefix):
		return fmt.Errorf("stack route %s: the sign-in routes are the core's", name)
	case rt.Level() == stacksdk.AccessStepUp && rt.Method == http.MethodGet:
		return fmt.Errorf("stack route %s: a read cannot need the step-up, which only guards writes", name)
	case rt.Level() == stacksdk.AccessStepUp && len(rt.Params()) > 0:
		return fmt.Errorf("stack route %s: a step-up route cannot have a wildcard", name)
	}
	return checkRouteAction(rt, canMint)
}

// checkRouteAction holds a route that names an action to what the grant needs. The SDK checks the shape (step-up write, no wildcard); the
// core checks the name against the catalogue it owns.
func checkRouteAction(rt stacksdk.RouteSpec, canMint bool) error {
	if rt.Action == "" {
		return nil
	}
	name := rt.Method + " " + rt.Path
	if _, err := catalog.Lookup(rt.Action); err != nil {
		return fmt.Errorf("stack route %s: %q is not an action of the privileged helper", name, rt.Action)
	}
	switch {
	case rt.Level() != stacksdk.AccessStepUp || rt.Method == http.MethodGet || len(rt.Params()) > 0:
		return fmt.Errorf("stack route %s: a route that names an action must be a step-up write with no wildcard", name)
	case !canMint:
		return fmt.Errorf("stack route %s: it names an action but the core has no way to mint a grant", name)
	}
	return nil
}

// StackStepUpRoutes lists the routes a stack declares with AccessStepUp, for AuthenticateWith.
func StackStepUpRoutes(m stacksdk.Manifest) []StepUpRoute {
	var out []StepUpRoute
	for _, rt := range m.Routes {
		if rt.Level() == stacksdk.AccessStepUp {
			out = append(out, StepUpRoute{Method: rt.Method, Path: rt.Path})
		}
	}
	return out
}

func stackHandler(s stacksdk.Stack, rt stacksdk.RouteSpec, minter GrantMinter) http.HandlerFunc {
	params := rt.Params()
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := stackRequest(w, r, params)
		if !ok {
			return
		}
		if rt.Action != "" && !attachGrant(w, &req, rt.Action, minter) {
			return
		}
		resp, err := s.Handle(r.Context(), req)
		if err != nil {
			WriteJSONError(w, statusOf(stacksdk.CodeOf(err)), messageOf(err))
			return
		}
		if resp.Status < 100 || resp.Status > 599 {
			WriteJSONError(w, http.StatusInternalServerError, "the stack answered with no valid status")
			return
		}
		if resp.ContentType != "" {
			w.Header().Set("Content-Type", resp.ContentType)
		}
		w.WriteHeader(resp.Status)
		w.Write(resp.Body)
	}
}

// stackRequest turns an HTTP request into the plain-data request a stack sees: the path wildcards and the body, bounded. It writes the
// refusal and returns false when the body is too large or cannot be read.
func stackRequest(w http.ResponseWriter, r *http.Request, params []string) (stacksdk.Request, bool) {
	req := stacksdk.Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, ContentType: r.Header.Get("Content-Type")}
	if len(params) > 0 {
		req.Params = make(map[string]string, len(params))
		for _, p := range params {
			req.Params[p] = r.PathValue(p)
		}
	}
	if r.Body == nil || r.Body == http.NoBody {
		return req, true
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteJSONError(w, http.StatusRequestEntityTooLarge, "the request body is too large")
		} else {
			WriteJSONError(w, http.StatusBadRequest, "the request body could not be read")
		}
		return req, false
	}
	req.Body = body
	return req, true
}

// attachGrant makes the request of a privileged route one the helper's catalogue accepts: the body must be exactly the action's
// parameters (strict decode, scope check), and only then is a grant minted, bound to the action and the canonical parameters, and put in
// the request. The stack never sees a body the catalogue would refuse. It writes the refusal and returns false when it cannot.
func attachGrant(w http.ResponseWriter, req *stacksdk.Request, action string, minter GrantMinter) bool {
	a, err := catalog.Lookup(action)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "the route names an action that does not exist")
		return false
	}
	prepared, err := a.Prepare(req.Body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, catalog.ErrScope) {
			status = http.StatusForbidden
		}
		WriteJSONError(w, status, privilegedRefusal(err))
		return false
	}
	grant, err := minter.Mint(action, prepared.Canonical)
	if err != nil {
		WriteJSONError(w, http.StatusServiceUnavailable, "too many privileged requests are waiting; try again shortly")
		return false
	}
	req.Grant = grant
	return true
}

// privilegedRefusal is the fixed sentence a caller sees for parameters the catalogue refused: what is wrong, never an echo of the input.
func privilegedRefusal(err error) string {
	if errors.Is(err, catalog.ErrScope) {
		return "the request is outside what this action may do"
	}
	return "the request body is not valid for this action"
}

// statusOf maps the code a stack failed with to the HTTP status the core's own handlers use for the same kind of refusal.
func statusOf(c stacksdk.Code) int {
	switch c {
	case stacksdk.CodeInvalid:
		return http.StatusBadRequest
	case stacksdk.CodeNotFound:
		return http.StatusNotFound
	case stacksdk.CodeDenied:
		return http.StatusForbidden
	case stacksdk.CodeUnavailable:
		return http.StatusServiceUnavailable
	case stacksdk.CodeTimeout:
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

// messageOf is the text the caller sees: the stack's own message for an *Error, else the error's text.
func messageOf(err error) string {
	var e *stacksdk.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return err.Error()
}
