package proxy

import (
	"fmt"
	"llm-proxy/models"
	"net/http"
	"sync"
)

type Server struct {
	manager    LLMProxyManager
	config     *models.Config
	configPath string
	modelDir   string
	configMu   sync.Mutex
}

var reverseProxyFactory = func(target string) http.Handler {
	return NewReverseProxy(target)
}

func NewServer(mgr LLMProxyManager, cfg *models.Config, configPath string) *Server {
	dir := ""
	if cfg != nil {
		dir = cfg.ModelDir
	}

	return &Server{
		manager:    mgr,
		config:     cfg,
		configPath: configPath,
		modelDir:   dir,
	}
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
