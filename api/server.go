package api

import (
	"fmt"
	"net/http"
)

type Server struct {
	Mgr *LLMProxyClient
}

func (s *Server) ChatHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		http.Error(w, "missing X-Model-Name header", http.StatusBadRequest)
		return
	}

	// Ensure the correct model is running
	port, err := s.Mgr.EnsureModel(model)
	if err != nil {
		http.Error(w, "failed to load model: "+err.Error(), http.StatusInternalServerError)
		return
	}

	target := fmt.Sprintf("http://127.0.0.1:%d", port)
	rp := NewReverseProxy(target)

	// Touch model for idle-timeout
	s.Mgr.RecordActivity(model)

	// STREAM response from llama.cpp → client
	rp.ServeHTTP(w, r)
}
