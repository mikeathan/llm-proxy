package proxy

import (
	"fmt"
	"net/http"
)

type Server struct {
	Mgr LLMProxyManager
}

var reverseProxyFactory = func(target string) http.Handler {
	return NewReverseProxy(target)
}

func (s *Server) ChatHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		http.Error(w, "missing X-Model-Name", http.StatusBadRequest)
		return
	}

	port, err := s.Mgr.EnsureModel(model)
	if err == ErrModelStarting {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-LLM-Status", "starting")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"starting"}`)
		return
	}

	if err != nil {
		http.Error(w, "model error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ready → Reverse proxy to llama.cpp
	s.Mgr.RecordActivity(model)

	target := fmt.Sprintf("http://127.0.0.1:%d", port)
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
