package api

import (
	"net/http"
)

const anyMethod = "*"

type Router struct {
	mux    *http.ServeMux
	routes map[string]*route
}

type route struct {
	handlers         map[string]http.Handler
	methodNotAllowed http.Handler
}

type RouteOption func(*route)

func WithMethodNotAllowed(handler http.Handler) RouteOption {
	return func(r *route) {
		r.methodNotAllowed = handler
	}
}

func MethodNotAllowedJSON(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func MethodNotAllowedText(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func NewRouter() *Router {
	return &Router{
		mux:    http.NewServeMux(),
		routes: make(map[string]*route),
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *Router) Handle(method, path string, handler http.Handler, opts ...RouteOption) {
	rt, ok := r.routes[path]
	if !ok {
		rt = &route{handlers: make(map[string]http.Handler)}
		r.routes[path] = rt
		r.mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if h := rt.handlers[req.Method]; h != nil {
				h.ServeHTTP(w, req)
				return
			}
			if h := rt.handlers[anyMethod]; h != nil {
				h.ServeHTTP(w, req)
				return
			}
			methodNotAllowed := rt.methodNotAllowed
			if methodNotAllowed == nil {
				methodNotAllowed = http.HandlerFunc(MethodNotAllowedText)
			}
			methodNotAllowed.ServeHTTP(w, req)
		}))
	}

	for _, opt := range opts {
		opt(rt)
	}

	rt.handlers[method] = handler
}

func (r *Router) Any(path string, handler http.Handler, opts ...RouteOption) {
	r.Handle(anyMethod, path, handler, opts...)
}

func (r *Router) Get(path string, handler http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodGet, path, handler, opts...)
}

func (r *Router) Post(path string, handler http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodPost, path, handler, opts...)
}

func (r *Router) Put(path string, handler http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodPut, path, handler, opts...)
}

func (r *Router) Delete(path string, handler http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodDelete, path, handler, opts...)
}

func (r *Router) Patch(path string, handler http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodPatch, path, handler, opts...)
}
