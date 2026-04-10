package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"llm-proxy/internal/core/llm"
)

type ProxyHandlers struct {
	runtime RuntimeService
}

func NewProxyHandlers(runtime RuntimeService) *ProxyHandlers {
	return &ProxyHandlers{runtime: runtime}
}

var reverseProxyFactory = func(target string) http.Handler {
	return NewReverseProxy(target)
}

func SetReverseProxyFactory(f func(string) http.Handler) func() {
	orig := reverseProxyFactory
	reverseProxyFactory = f
	return func() {
		reverseProxyFactory = orig
	}
}

func (h *ProxyHandlers) EnsureModelProxyHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		http.Error(w, "missing X-Model-Name", http.StatusBadRequest)
		return
	}

	mi, err := h.runtime.EnsureModel(r.Context(), model)
	if err == llm.ErrModelStarting {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-LLM-Status", "starting")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"starting"}`))
		return
	}

	if err != nil {
		http.Error(w, "model error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.runtime.RecordActivity(model)
	
	target := mi.URL
	isCloud := true
	if target == "" {
		target = fmt.Sprintf("http://%s:%d", mi.Host, mi.Port)
		isCloud = false
	}

	rp := reverseProxyFactory(target)

	// If we have custom headers (e.g. for cloud providers), we need to inject them.
	inner := rp
	rp = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For cloud providers, the mi.URL now contains the FULL endpoint (e.g. .../v1/chat/completions).
		// We must override the incoming path (/v1/chat/completions) with the target's path.
		if isCloud {
			// For cloud providers, the target already contains the full path (e.g., /v1/chat/completions).
			// We MUST clear the request path so the ReverseProxy doesn't double it.
			r.URL.Path = ""
			r.URL.RawPath = ""
		}

		for k, vv := range mi.Headers {
			for _, v := range vv {
				r.Header.Set(k, v)
			}
		}
		inner.ServeHTTP(w, r)
	})

	rp.ServeHTTP(w, r)
}

func NewReverseProxy(target string) *httputil.ReverseProxy {
	u, _ := url.Parse(target)

	proxy := httputil.NewSingleHostReverseProxy(u)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
	}

	return proxy
}
