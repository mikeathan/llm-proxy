package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-proxy-stacks/sdk"
)

// A stack declares its HTTP routes as data in its manifest (stacksdk.RouteSpec) and answers each with a plain-data Request and Response. The
// core mounts the routes here, under the same method and path the stack declares, so the frontend and the JSON are unchanged. Every route is
// inside the admin API, so Authenticate signs the caller in before the stack is called; the stack cannot lower that, and the only way to
// raise it is to declare AccessStepUp, which AuthenticateWith enforces with the core's own step-up check (plan P3b D5).

// MountStackRoutes registers each route the stack declares on mux. It refuses, before registering anything, a route the core could not keep
// safe: one outside /admin/api/, one under the sign-in routes (which need no credential), a step-up read (the step-up check skips reads, so
// it would protect nothing), a step-up route with a wildcard (the step-up table matches exact paths), and one already registered.
func MountStackRoutes(mux *http.ServeMux, s stacksdk.Stack) (err error) {
	routes := s.Manifest().Routes
	for _, rt := range routes {
		if err := checkStackRoute(rt); err != nil {
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
		mux.HandleFunc(rt.Method+" "+rt.Path, stackHandler(s, rt))
	}
	return nil
}

func checkStackRoute(rt stacksdk.RouteSpec) error {
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

func stackHandler(s stacksdk.Stack, rt stacksdk.RouteSpec) http.HandlerFunc {
	params := rt.Params()
	return func(w http.ResponseWriter, r *http.Request) {
		req := stacksdk.Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, ContentType: r.Header.Get("Content-Type")}
		if len(params) > 0 {
			req.Params = make(map[string]string, len(params))
			for _, p := range params {
				req.Params[p] = r.PathValue(p)
			}
		}
		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
			if err != nil {
				var tooBig *http.MaxBytesError
				if errors.As(err, &tooBig) {
					WriteJSONError(w, http.StatusRequestEntityTooLarge, "the request body is too large")
				} else {
					WriteJSONError(w, http.StatusBadRequest, "the request body could not be read")
				}
				return
			}
			req.Body = body
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
