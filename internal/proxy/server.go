package proxy

import (
	"fmt"
	"net/http"
)

type Server struct {
	manager LLMProxyManager
}

var reverseProxyFactory = func(target string) http.Handler {
	return NewReverseProxy(target)
}

func NewServer(mgr LLMProxyManager) *Server {
	return &Server{manager: mgr}
}

func (s *Server) ChatHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		http.Error(w, "missing X-Model-Name", http.StatusBadRequest)
		return
	}

	mi, err := s.manager.EnsureModel(model)
	if err == ErrModelStarting {
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

	s.manager.RecordActivity(model)

	target := fmt.Sprintf("http://%s:%d", mi.Host, mi.Port)
	rp := reverseProxyFactory(target)
	rp.ServeHTTP(w, r)
}

func SetReverseProxyFactory(f func(string) http.Handler) func() {
	orig := reverseProxyFactory
	reverseProxyFactory = f
	return func() {
		reverseProxyFactory = orig
	}
}
