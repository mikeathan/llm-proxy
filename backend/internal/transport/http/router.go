package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime/debug"

	"llm-proxy/internal/platform/logging"
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
	WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
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
			recoverHandler(methodNotAllowed).ServeHTTP(w, req)
		}))
	}

	for _, opt := range opts {
		opt(rt)
	}

	rt.handlers[method] = recoverHandler(handler)
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

// recoverHandler contains panics from a downstream handler so a single bad
// request cannot crash the whole server. The panic is logged with a stack
// trace; if the handler has not yet written a response, a generic 500 is
// returned. If the response was already started (e.g. a flushed SSE stream),
// the connection is left as-is and the client reconnects — the EventBus still
// self-cleans via the handler's deferred Unsubscribe during the unwind.
func recoverHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &panicRecorder{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				logging.Error("http handler panic",
					"method", r.Method,
					"path", r.URL.Path,
					"error", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
				)
				if !rw.wrote {
					rw.WriteHeader(http.StatusInternalServerError)
					_, _ = rw.Write([]byte("internal server error"))
				}
			}
		}()
		h.ServeHTTP(rw, r)
	})
}

// panicRecorder wraps http.ResponseWriter, tracking whether any status or body
// has been written so the recovery path can avoid clobbering an in-flight
// response. It forwards Flusher, Hijacker and ReaderFrom so streaming and
// upgrade-style handlers keep working.
type panicRecorder struct {
	http.ResponseWriter
	wrote bool
}

func (w *panicRecorder) WriteHeader(status int) {
	if !w.wrote {
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *panicRecorder) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *panicRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *panicRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

func (w *panicRecorder) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	// Fallback: copy through our Write (which marks wrote). Use an adapter
	// whose dynamic type does NOT implement ReaderFrom, otherwise io.Copy
	// would re-invoke this method and recurse.
	return io.Copy(prWriter{w: w}, src)
}

// prWriter adapts panicRecorder to a plain io.Writer so io.Copy's internal
// ReaderFrom check does not recurse into panicRecorder.ReadFrom.
type prWriter struct {
	w *panicRecorder
}

func (p prWriter) Write(b []byte) (int, error) {
	return p.w.Write(b)
}
