package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"llm-proxy/models"
	"llm-proxy/utils"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type adminModelView struct {
	Name         string   `json:"name"`
	Filename     string   `json:"filename"`
	ResolvedPath string   `json:"resolved_path"`
	Args         []string `json:"args"`
	Port         int      `json:"port"`
	Endpoint     string   `json:"endpoint"`
	Active       bool     `json:"active"`
	Ready        bool     `json:"ready"`
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
	Name         string `json:"name"`
	Filename     string `json:"filename"`
	ResolvedPath string `json:"resolved_path"`
}

type adminStateResponse struct {
	Models    []adminModelView      `json:"models"`
	Available []adminAvailableModel `json:"available,omitempty"`
	NextPort  int                   `json:"next_port"`
	Active    *adminActiveModel     `json:"active,omitempty"`
	Config    adminConfigView       `json:"config"`
}

type adminConfigView struct {
	ModelDir     string `json:"model_dir"`
	LlamaBinary  string `json:"llama_binary"`
	ModelHost    string `json:"model_host"`
	IdleTimeoutS int    `json:"idle_timeout_seconds"`
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

type adminLogsResponse struct {
	Running   bool      `json:"running"`
	Name      string    `json:"name,omitempty"`
	Ready     bool      `json:"ready,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Logs      string    `json:"logs"`
}

type hostMetrics struct {
	Load1          float64   `json:"load1"`
	Load5          float64   `json:"load5"`
	Load15         float64   `json:"load15"`
	LoadPct        float64   `json:"load_percent"`
	MemTotalMB     float64   `json:"mem_total_mb"`
	MemFreeMB      float64   `json:"mem_free_mb"`
	MemAvailableMB float64   `json:"mem_available_mb"`
	MemUsedMB      float64   `json:"mem_used_mb"`
	Timestamp      time.Time `json:"timestamp"`
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
			ModelDir:     s.modelDir,
			LlamaBinary:  s.currentBinary(),
			ModelHost:    host,
			IdleTimeoutS: s.currentIdleTimeout(),
		},
	}

	for _, mc := range models {
		state.Models = append(state.Models, adminModelView{
			Name:         mc.Name,
			Filename:     mc.Filename,
			ResolvedPath: mc.Path,
			Args:         mc.Args,
			Port:         mc.Port,
			Endpoint:     fmt.Sprintf("http://%s:%d", host, mc.Port),
			Active:       mc.Name == activeName,
			Ready:        mc.Name == activeName && activeDetails != nil && activeDetails.Ready,
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
			ModelDir:     s.modelDir,
			LlamaBinary:  s.currentBinary(),
			ModelHost:    s.manager.ModelHost(),
			IdleTimeoutS: s.currentIdleTimeout(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	case http.MethodPut:
		var req struct {
			ModelDir    string `json:"model_dir"`
			LlamaBinary string `json:"llama_binary"`
			ModelHost   string `json:"model_host"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}

		if req.ModelDir != "" {
			s.modelDir = req.ModelDir
		}
		if req.LlamaBinary != "" {
			s.manager.SetBinary(req.LlamaBinary)
		}
		if req.ModelHost != "" {
			s.manager.SetModelHost(req.ModelHost)
		}

		if s.config != nil {
			s.configMu.Lock()
			if req.ModelDir != "" {
				s.config.ModelDir = req.ModelDir
			}
			if req.LlamaBinary != "" {
				s.config.Server.LlamaServerBinary = req.LlamaBinary
			}
			if req.ModelHost != "" {
				s.config.Server.ModelHost = req.ModelHost
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

func (s *Server) AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	active := s.manager.ActiveInfo()
	resp := adminLogsResponse{
		Running: active != nil,
		Logs:    s.manager.ActiveLogs(),
	}

	if active != nil {
		resp.Name = active.Name
		resp.Ready = active.Ready
		resp.StartedAt = active.Started
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	load1, load5, load15, _ := readLoadAvg()
	mem := readMemInfo()
	pct := 0.0
	if cores := float64(runtime.NumCPU()); cores > 0 {
		pct = (load1 / cores) * 100
	}

	resp := hostMetrics{
		Load1:          load1,
		Load5:          load5,
		Load15:         load15,
		LoadPct:        pct,
		MemTotalMB:     mem.totalMB,
		MemFreeMB:      mem.freeMB,
		MemAvailableMB: mem.availMB,
		MemUsedMB:      mem.usedMB,
		Timestamp:      time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Filename string   `json:"filename"`
		Path     string   `json:"path"` // legacy support
		Args     []string `json:"args"`
		Port     int      `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	filename := strings.TrimSpace(req.Filename)
	if filename == "" && req.Path != "" {
		filename = filepath.Base(req.Path)
	}
	if filename == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model filename")
		return
	}

	if req.Name == "" {
		ext := filepath.Ext(filename)
		req.Name = strings.TrimSuffix(filename, ext)
	}

	fullPath := s.resolveModelPath(filename, req.Path)
	if _, err := os.Stat(fullPath); err != nil {
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
		Name:     req.Name,
		Filename: filename,
		Path:     fullPath,
		Args:     args,
		Port:     req.Port,
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
		Name:     req.Name,
		Filename: filename,
		Args:     append([]string{}, req.Args...),
		Port:     req.Port,
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
		Name     string   `json:"name"`
		Filename string   `json:"filename"`
		Path     string   `json:"path"` // legacy support
		Args     []string `json:"args"`
		Port     int      `json:"port"`
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

	if req.Filename == "" && req.Path != "" {
		req.Filename = filepath.Base(req.Path)
	}
	if req.Filename == "" {
		req.Filename = existing.Filename
	}
	if req.Port == 0 {
		req.Port = existing.Port
	}
	if len(req.Args) == 0 {
		req.Args = existing.Args
	}

	args := req.Args
	fullPath := s.resolveModelPath(req.Filename, req.Path)
	if _, err := os.Stat(fullPath); err != nil {
		writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
		return
	}

	runtimeCfg := models.ModelConfig{
		Name:     req.Name,
		Filename: req.Filename,
		Path:     fullPath,
		Args:     args,
		Port:     req.Port,
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
		Name:     req.Name,
		Filename: req.Filename,
		Args:     append([]string{}, req.Args...),
		Port:     req.Port,
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

type memSnapshot struct {
	totalMB float64
	freeMB  float64
	availMB float64
	usedMB  float64
}

func readLoadAvg() (float64, float64, float64, error) {
	data, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("unexpected loadavg format")
	}
	parse := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	return parse(fields[0]), parse(fields[1]), parse(fields[2]), nil
}

func readMemInfo() memSnapshot {
	data, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return memSnapshot{}
	}
	lines := strings.Split(string(data), "\n")
	toMB := func(kb float64) float64 { return kb / 1024.0 }
	info := make(map[string]float64)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(fields[1], 64)
		info[fields[0]] = v
	}
	total := toMB(info["MemTotal:"])
	free := toMB(info["MemFree:"])
	avail := toMB(info["MemAvailable:"])
	used := total - avail
	return memSnapshot{
		totalMB: total,
		freeMB:  free,
		availMB: avail,
		usedMB:  used,
	}
}

func (s *Server) resolveModelPath(filename, explicitPath string) string {
	if explicitPath != "" && filepath.IsAbs(explicitPath) {
		return explicitPath
	}
	if filename == "" && explicitPath != "" {
		return explicitPath
	}
	if filepath.IsAbs(filename) {
		return filename
	}
	if s.modelDir != "" {
		return filepath.Join(s.modelDir, filename)
	}
	if explicitPath != "" {
		return explicitPath
	}
	return filename
}

func (s *Server) discoverModelFiles(current []models.ModelConfig) []adminAvailableModel {
	if s.modelDir == "" {
		return nil
	}

	if info, err := os.Stat(s.modelDir); err != nil || !info.IsDir() {
		return nil
	}

	seenNames := make(map[string]struct{}, len(current))
	seenPaths := make(map[string]struct{}, len(current))
	for _, m := range current {
		seenNames[m.Name] = struct{}{}
		if m.Path != "" {
			seenPaths[m.Path] = struct{}{}
		}
	}

	var found []adminAvailableModel
	_ = filepath.WalkDir(s.modelDir, func(path string, d fs.DirEntry, err error) error {
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
			Name:         name,
			Filename:     d.Name(),
			ResolvedPath: fullPath,
		})
		return nil
	})

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
