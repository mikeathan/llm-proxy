package proxy

import (
	_ "embed"
	"net/http"
)

//go:embed admin_ui.html
var adminPage []byte

func (s *Server) AdminPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(adminPage)
}
