package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type adminModelView struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Args     []string `json:"args"`
	Port     int      `json:"port"`
	Endpoint string   `json:"endpoint"`
	Active   bool     `json:"active"`
	Ready    bool     `json:"ready"`
}

type adminActiveModel struct {
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	Port      int       `json:"port"`
	Ready     bool      `json:"ready"`
	StartedAt time.Time `json:"started_at"`
	LastUsed  time.Time `json:"last_used_at"`
}

type adminStateResponse struct {
	Models []adminModelView  `json:"models"`
	Active *adminActiveModel `json:"active,omitempty"`
}

type adminStartResponse struct {
	Status   string `json:"status"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint,omitempty"`
	Port     int    `json:"port,omitempty"`
}

type adminStopResponse struct {
	Status  string `json:"status"`
	Stopped string `json:"stopped,omitempty"`
}

func (s *Server) AdminStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models := s.manager.ListModels()
	host := s.manager.ModelHost()

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	var activeName string
	var activeDetails *adminActiveModel

	if ai := s.manager.ActiveInfo(); ai != nil {
		activeName = ai.Name
		activeDetails = &adminActiveModel{
			Name:      ai.Name,
			Endpoint:  fmt.Sprintf("http://%s:%d", host, ai.Port),
			Port:      ai.Port,
			Ready:     ai.Ready,
			StartedAt: ai.Started,
			LastUsed:  ai.LastUsed,
		}
	}

	state := adminStateResponse{
		Models: make([]adminModelView, 0, len(models)),
		Active: activeDetails,
	}

	for _, mc := range models {
		state.Models = append(state.Models, adminModelView{
			Name:     mc.Name,
			Path:     mc.Path,
			Args:     mc.Args,
			Port:     mc.Port,
			Endpoint: fmt.Sprintf("http://%s:%d", host, mc.Port),
			Active:   mc.Name == activeName,
			Ready:    mc.Name == activeName && activeDetails != nil && activeDetails.Ready,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *Server) AdminStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = r.URL.Query().Get("name")
	}

	if req.Name == "" {
		http.Error(w, "missing model name", http.StatusBadRequest)
		return
	}

	mi, err := s.manager.EnsureModel(req.Name)
	if err == ErrModelStarting {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(adminStartResponse{
			Status: "starting",
			Model:  req.Name,
		})
		return
	}

	if err != nil {
		http.Error(w, "unable to start model: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.manager.RecordActivity(req.Name)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminStartResponse{
		Status:   "ready",
		Model:    req.Name,
		Endpoint: fmt.Sprintf("http://%s:%d", s.manager.ModelHost(), mi.Port),
		Port:     mi.Port,
	})
}

func (s *Server) AdminStopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	active := s.manager.ActiveInfo()

	if err := s.manager.StopActive(); err != nil {
		http.Error(w, "unable to stop model: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := adminStopResponse{Status: "idle"}
	if active != nil {
		resp.Status = "stopped"
		resp.Stopped = active.Name
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
