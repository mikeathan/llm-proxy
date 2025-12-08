package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"llm-proxy/models"
	"llm-proxy/utils"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type adminAvailableModel struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type adminStateResponse struct {
	Models    []adminModelView      `json:"models"`
	Available []adminAvailableModel `json:"available,omitempty"`
	NextPort  int                   `json:"next_port"`
	Active    *adminActiveModel     `json:"active,omitempty"`
	Config    adminConfigView       `json:"config"`
}

type adminConfigView struct {
	ModelDirs    []string `json:"model_dirs"`
	LlamaBinary  string   `json:"llama_binary"`
	ModelHost    string   `json:"model_host"`
	IdleTimeoutS int      `json:"idle_timeout_seconds"`
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
	Error   string `json:"error,omitempty"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) currentBinary() string {
	if s.config != nil && s.config.Server.LlamaServerBinary != "" {
		return s.config.Server.LlamaServerBinary
	}
	return "llama-server"
}

func (s *Server) currentIdleTimeout() int {
	if s.config != nil {
		return s.config.Server.IdleTimeoutSecs
	}
	return 0
}

func (s *Server) AdminStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models := s.manager.ListModels()
	host := s.manager.ModelHost()
	var available []adminAvailableModel
	if v := strings.ToLower(r.URL.Query().Get("available")); v == "1" || v == "true" {
		available = s.discoverModelFiles(models)
	}

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

	activePort := 0
	if activeDetails != nil {
		activePort = activeDetails.Port
	}
	nextPort := nextAvailablePort(models, activePort)

	state := adminStateResponse{
		Models:    make([]adminModelView, 0, len(models)),
		Available: available,
		NextPort:  nextPort,
		Active:    activeDetails,
		Config: adminConfigView{
			ModelDirs:    append([]string{}, s.modelDirs...),
			LlamaBinary:  s.currentBinary(),
			ModelHost:    host,
			IdleTimeoutS: s.currentIdleTimeout(),
		},
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		writeJSONError(w, http.StatusBadRequest, "missing model name")
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
		writeJSONError(w, http.StatusInternalServerError, "unable to start model: "+err.Error())
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

func (s *Server) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleAddModel(w, r)
	case http.MethodPut:
		s.handleUpdateModel(w, r)
	case http.MethodDelete:
		s.handleDeleteModel(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := adminConfigView{
			ModelDirs:    append([]string{}, s.modelDirs...),
			LlamaBinary:  s.currentBinary(),
			ModelHost:    s.manager.ModelHost(),
			IdleTimeoutS: s.currentIdleTimeout(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	case http.MethodPut:
		var req struct {
			ModelDirs   []string `json:"model_dirs"`
			LlamaBinary string   `json:"llama_binary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}

		if len(req.ModelDirs) > 0 {
			s.modelDirs = req.ModelDirs
		}
		if req.LlamaBinary != "" {
			s.manager.SetBinary(req.LlamaBinary)
		}

		if s.config != nil {
			s.configMu.Lock()
			if len(req.ModelDirs) > 0 {
				s.config.ModelDirs = req.ModelDirs
			}
			if req.LlamaBinary != "" {
				s.config.Server.LlamaServerBinary = req.LlamaBinary
			}
			_ = utils.SaveConfig(s.configPath, s.config)
			s.configMu.Unlock()
		}

		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) AdminStopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	active := s.manager.ActiveInfo()

	err := s.manager.StopActive()

	resp := adminStopResponse{Status: "idle"}
	if err != nil {
		resp.Error = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	} else if active != nil {
		resp.Status = "stopped"
		resp.Stopped = active.Name
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string   `json:"name"`
		Path string   `json:"path"`
		Args []string `json:"args"`
		Port int      `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model path")
		return
	}

	if req.Name == "" {
		ext := filepath.Ext(req.Path)
		req.Name = strings.TrimSuffix(filepath.Base(req.Path), ext)
	}

	if _, err := os.Stat(req.Path); err != nil {
		writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
		return
	}

	if req.Port == 0 {
		active := s.manager.ActiveInfo()
		activePort := 0
		if active != nil {
			activePort = active.Port
		}
		req.Port = nextAvailablePort(s.manager.ListModels(), activePort)
	}

	args := req.Args
	if s.config != nil && len(s.config.Server.DefaultArgs) > 0 {
		args = append(append([]string{}, s.config.Server.DefaultArgs...), req.Args...)
	}

	runtimeCfg := models.ModelConfig{
		Name: req.Name,
		Path: req.Path,
		Args: args,
		Port: req.Port,
	}

	if err := s.manager.AddModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrModelExists) {
			status = http.StatusConflict
		}
		writeJSONError(w, status, "unable to add model: "+err.Error())
		return
	}

	persistCfg := models.ModelConfig{
		Name: req.Name,
		Path: req.Path,
		Args: append([]string{}, req.Args...),
		Port: req.Port,
	}

	if err := s.persistModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "saved model but failed to persist config: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runtimeCfg)
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string   `json:"name"`
		Path string   `json:"path"`
		Args []string `json:"args"`
		Port int      `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	var existing models.ModelConfig
	found := false
	for _, m := range s.manager.ListModels() {
		if m.Name == req.Name {
			existing = m
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown model")
		return
	}

	if req.Path == "" {
		req.Path = existing.Path
	}
	if req.Port == 0 {
		req.Port = existing.Port
	}
	if len(req.Args) == 0 {
		req.Args = existing.Args
	}

	args := req.Args
	runtimeCfg := models.ModelConfig{
		Name: req.Name,
		Path: req.Path,
		Args: args,
		Port: req.Port,
	}

	if err := s.manager.UpdateModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to update model: "+err.Error())
		return
	}

	persistCfg := models.ModelConfig{
		Name: req.Name,
		Path: req.Path,
		Args: append([]string{}, req.Args...),
		Port: req.Port,
	}

	if err := s.persistReplaceModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "updated model but failed to persist config: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runtimeCfg)
}

func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		name = req.Name
	}

	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	if err := s.manager.RemoveModel(name); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to delete model: "+err.Error())
		return
	}

	if err := s.persistDeleteModel(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "deleted model but failed to persist config: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) persistModel(cfg models.ModelConfig) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	for _, existing := range s.config.Models {
		if existing.Name == cfg.Name {
			return nil
		}
	}

	s.config.Models = append(s.config.Models, cfg)
	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) persistReplaceModel(cfg models.ModelConfig) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	replaced := false
	for i, m := range s.config.Models {
		if m.Name == cfg.Name {
			s.config.Models[i] = cfg
			replaced = true
			break
		}
	}
	if !replaced {
		s.config.Models = append(s.config.Models, cfg)
	}

	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) persistDeleteModel(name string) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	out := s.config.Models[:0]
	for _, m := range s.config.Models {
		if m.Name != name {
			out = append(out, m)
		}
	}
	s.config.Models = out

	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) discoverModelFiles(current []models.ModelConfig) []adminAvailableModel {
	if len(s.modelDirs) == 0 {
		return nil
	}

	seenNames := make(map[string]struct{}, len(current))
	seenPaths := make(map[string]struct{}, len(current))
	for _, m := range current {
		seenNames[m.Name] = struct{}{}
		seenPaths[m.Path] = struct{}{}
	}

	var found []adminAvailableModel
	for _, dir := range s.modelDirs {
		dir := dir
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext != ".gguf" {
				return nil
			}
			fullPath := path
			if _, ok := seenPaths[fullPath]; ok {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), ext)
			if _, ok := seenNames[name]; ok {
				return nil
			}
			found = append(found, adminAvailableModel{
				Name: name,
				Path: fullPath,
			})
			return nil
		})
	}

	sort.Slice(found, func(i, j int) bool {
		return found[i].Name < found[j].Name
	})

	return found
}

func nextAvailablePort(models []models.ModelConfig, activePort int) int {
	if activePort != 0 {
		return activePort
	}
	port := 8081
	for _, m := range models {
		if m.Port >= port {
			port = m.Port + 1
		}
	}
	return port
}
