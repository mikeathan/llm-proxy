package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"llm-proxy/internal/proxy"
	"llm-proxy/models"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AdminHandlers struct {
	server *proxy.Server
}

func NewAdminHandlers(server *proxy.Server) *AdminHandlers {
	return &AdminHandlers{server: server}
}

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
	GPUProvider  string `json:"gpu_provider,omitempty"`
	GPUBinary    string `json:"gpu_binary,omitempty"`
	GPUIndex     int    `json:"gpu_index,omitempty"`
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

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *AdminHandlers) AdminStateHandler(w http.ResponseWriter, r *http.Request) {
	mgr := h.server.Manager()
	modelsList := mgr.ListModels()
	host := mgr.ModelHost()
	var available []adminAvailableModel
	if v := strings.ToLower(r.URL.Query().Get("available")); v == "1" || v == "true" {
		available = discoverModelFiles(h.server.ModelDir(), modelsList)
	}

	sort.Slice(modelsList, func(i, j int) bool {
		return modelsList[i].Name < modelsList[j].Name
	})

	var activeName string
	var activeDetails *adminActiveModel
	if ai := mgr.ActiveInfo(); ai != nil {
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
	nextPort := nextAvailablePort(modelsList, activePort)
	gpuCfg := h.server.GPUConfig()

	state := adminStateResponse{
		Models:    make([]adminModelView, 0, len(modelsList)),
		Available: available,
		NextPort:  nextPort,
		Active:    activeDetails,
		Config: adminConfigView{
			ModelDir:     h.server.ModelDir(),
			LlamaBinary:  h.server.CurrentBinary(),
			ModelHost:    host,
			IdleTimeoutS: h.server.CurrentIdleTimeout(),
			GPUProvider:  gpuCfg.Provider,
			GPUBinary:    gpuCfg.Binary,
			GPUIndex:     gpuCfg.Index,
		},
	}

	for _, mc := range modelsList {
		filename := mc.Filename
		if filename == "" && mc.Path != "" {
			filename = filepath.Base(mc.Path)
		}
		state.Models = append(state.Models, adminModelView{
			Name:         mc.Name,
			Filename:     filename,
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

func (h *AdminHandlers) AdminStartHandler(w http.ResponseWriter, r *http.Request) {
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

	mgr := h.server.Manager()
	mi, err := mgr.EnsureModel(req.Name)
	if err == proxy.ErrModelStarting {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(adminStartResponse{Status: "starting", Model: req.Name})
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "model error: "+err.Error())
		return
	}

	resp := adminStartResponse{
		Status:   "started",
		Model:    req.Name,
		Endpoint: fmt.Sprintf("http://%s:%d", mi.Host, mi.Port),
		Port:     mi.Port,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminHandlers) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleAddModel(w, r)
}

func (h *AdminHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	gpuCfg := h.server.GPUConfig()
	cfg := adminConfigView{
		ModelDir:     h.server.ModelDir(),
		LlamaBinary:  h.server.CurrentBinary(),
		ModelHost:    h.server.Manager().ModelHost(),
		IdleTimeoutS: h.server.CurrentIdleTimeout(),
		GPUProvider:  gpuCfg.Provider,
		GPUBinary:    gpuCfg.Binary,
		GPUIndex:     gpuCfg.Index,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

func (h *AdminHandlers) AdminStopHandler(w http.ResponseWriter, r *http.Request) {
	mgr := h.server.Manager()
	active := mgr.ActiveInfo()

	err := mgr.StopActive()

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

func (h *AdminHandlers) AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	mgr := h.server.Manager()
	active := mgr.ActiveInfo()
	resp := adminLogsResponse{
		Running: active != nil,
		Logs:    mgr.ActiveLogs(),
	}

	if active != nil {
		resp.Name = active.Name
		resp.Ready = active.Ready
		resp.StartedAt = active.Started
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminHandlers) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	resp := h.server.MetricsSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminHandlers) AdminUpdateModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateModel(w, r)
}

func (h *AdminHandlers) AdminDeleteModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteModel(w, r)
}

func (h *AdminHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelDir    string `json:"model_dir"`
		LlamaBinary string `json:"llama_binary"`
		ModelHost   string `json:"model_host"`
		GPUProvider string `json:"gpu_provider"`
		GPUBinary   string `json:"gpu_binary"`
		GPUIndex    *int   `json:"gpu_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.ModelDir != "" {
		h.server.SetModelDir(req.ModelDir)
	}
	if req.LlamaBinary != "" {
		h.server.Manager().SetBinary(req.LlamaBinary)
	}
	if req.ModelHost != "" {
		h.server.Manager().SetModelHost(req.ModelHost)
	}
	gpuCfg := h.server.GPUConfig()
	if req.GPUProvider != "" {
		gpuCfg.Provider = req.GPUProvider
	}
	if req.GPUBinary != "" {
		gpuCfg.Binary = req.GPUBinary
	}
	if req.GPUIndex != nil {
		gpuCfg.Index = *req.GPUIndex
	}
	h.server.SetGPUConfig(gpuCfg)

	if err := h.server.UpdateConfig(func(cfg *models.Config) {
		if req.ModelDir != "" {
			cfg.ModelDir = req.ModelDir
		}
		if req.LlamaBinary != "" {
			cfg.Server.LlamaServerBinary = req.LlamaBinary
		}
		if req.ModelHost != "" {
			cfg.Server.ModelHost = req.ModelHost
		}
		if req.GPUProvider != "" || req.GPUBinary != "" || req.GPUIndex != nil {
			cfg.Metrics.GPU.Provider = gpuCfg.Provider
			cfg.Metrics.GPU.Binary = gpuCfg.Binary
			if req.GPUIndex != nil {
				cfg.Metrics.GPU.Index = gpuCfg.Index
			}
		}
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	h.server.RefreshMetricsService()

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandlers) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Filename string   `json:"filename"`
		Path     string   `json:"path"`
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

	fullPath := h.server.ResolveModelPath(filename, req.Path)
	if _, err := os.Stat(fullPath); err != nil {
		writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
		return
	}

	mgr := h.server.Manager()
	if req.Port == 0 {
		active := mgr.ActiveInfo()
		activePort := 0
		if active != nil {
			activePort = active.Port
		}
		req.Port = nextAvailablePort(mgr.ListModels(), activePort)
	}

	args := req.Args
	if defaults := h.server.DefaultArgs(); len(defaults) > 0 {
		args = append(append([]string{}, defaults...), req.Args...)
	}

	runtimeCfg := models.ModelConfig{
		Name:     req.Name,
		Filename: filename,
		Path:     fullPath,
		Args:     args,
		Port:     req.Port,
	}

	if err := mgr.AddModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, proxy.ErrModelExists) {
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

	if err := h.server.PersistModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "saved model but failed to persist config: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runtimeCfg)
}

func (h *AdminHandlers) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Filename string   `json:"filename"`
		Path     string   `json:"path"`
		Args     []string `json:"args"`
		Port     int      `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	mgr := h.server.Manager()
	var existing models.ModelConfig
	found := false
	for _, m := range mgr.ListModels() {
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
	fullPath := h.server.ResolveModelPath(req.Filename, req.Path)
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

	if err := mgr.UpdateModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, proxy.ErrUnknownModel) {
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

	if err := h.server.PersistReplaceModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "updated model but failed to persist config: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runtimeCfg)
}

func (h *AdminHandlers) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
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

	if err := h.server.Manager().RemoveModel(name); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, proxy.ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to delete model: "+err.Error())
		return
	}

	if err := h.server.PersistDeleteModel(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "deleted model but failed to persist config: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func discoverModelFiles(modelDir string, current []models.ModelConfig) []adminAvailableModel {
	if modelDir == "" {
		return nil
	}

	if info, err := os.Stat(modelDir); err != nil || !info.IsDir() {
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
	_ = filepath.WalkDir(modelDir, func(path string, d fs.DirEntry, err error) error {
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

func nextAvailablePort(modelsList []models.ModelConfig, activePort int) int {
	if activePort != 0 {
		return activePort
	}
	port := 8081
	for _, m := range modelsList {
		if m.Port >= port {
			port = m.Port + 1
		}
	}
	return port
}
