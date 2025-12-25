package api

import (
	"fmt"
	"llm-proxy/internal/proxy"
	"net/http"
)

type ProxyHandlers struct {
	server *proxy.Server
}

func NewProxyHandlers(server *proxy.Server) *ProxyHandlers {
	return &ProxyHandlers{server: server}
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

func (h *ProxyHandlers) ChatHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		http.Error(w, "missing X-Model-Name", http.StatusBadRequest)
		return
	}

	mgr := h.server.Manager()
	mi, err := mgr.EnsureModel(model)
	if err == proxy.ErrModelStarting {
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

	mgr.RecordActivity(model)

	target := fmt.Sprintf("http://%s:%d", mi.Host, mi.Port)
	rp := reverseProxyFactory(target)
	rp.ServeHTTP(w, r)
}
