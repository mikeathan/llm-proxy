package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"llm-proxy/internal/llm"
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

	target := fmt.Sprintf("http://%s:%d", mi.Host, mi.Port)
	rp := reverseProxyFactory(target)
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
