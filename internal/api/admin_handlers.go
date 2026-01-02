package api

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/models"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed admin_ui.html
var adminFS embed.FS

var adminTmpl = template.Must(
	template.ParseFS(adminFS, "admin_ui.html"),
)

type AdminView struct {
	Version   string
	Commit    string
	BuildDate string
}

type AdminHandlers struct {
	runtime   RuntimeService
	admin     AdminService
	logger    logging.Logger
	version   string
	commit    string
	buildDate string
}

func NewAdminHandlers(
	runtime RuntimeService,
	admin AdminService,
	logger logging.Logger,
	version string,
	commit string,
	buildDate string,
) *AdminHandlers {
	return &AdminHandlers{
		runtime:   runtime,
		admin:     admin,
		logger:    logger,
		version:   version,
		commit:    commit,
		buildDate: buildDate,
	}
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
	AppLogOK  bool      `json:"app_log_ok,omitempty"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *AdminHandlers) AdminStateHandler(w http.ResponseWriter, r *http.Request) {
	modelsList := h.runtime.ListModels()
	host := h.runtime.ModelHost()
	var available []adminAvailableModel
	if v := strings.ToLower(r.URL.Query().Get("available")); v == "1" || v == "true" {
		available = discoverModelFiles(h.admin.ModelDir(), modelsList)
	}

	sort.Slice(modelsList, func(i, j int) bool {
		return modelsList[i].Name < modelsList[j].Name
	})

	var activeName string
	var activeDetails *adminActiveModel
	if ai := h.runtime.ActiveInfo(); ai != nil {
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
	gpuCfg := h.admin.GPUConfig()

	state := adminStateResponse{
		Models:    make([]adminModelView, 0, len(modelsList)),
		Available: available,
		NextPort:  nextPort,
		Active:    activeDetails,
		Config: adminConfigView{
			ModelDir:     h.admin.ModelDir(),
			LlamaBinary:  h.admin.CurrentBinary(),
			ModelHost:    host,
			IdleTimeoutS: h.admin.CurrentIdleTimeout(),
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

	mi, err := h.runtime.EnsureModel(req.Name)
	if err == llm.ErrModelStarting {
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
	gpuCfg := h.admin.GPUConfig()
	cfg := adminConfigView{
		ModelDir:     h.admin.ModelDir(),
		LlamaBinary:  h.admin.CurrentBinary(),
		ModelHost:    h.runtime.ModelHost(),
		IdleTimeoutS: h.admin.CurrentIdleTimeout(),
		GPUProvider:  gpuCfg.Provider,
		GPUBinary:    gpuCfg.Binary,
		GPUIndex:     gpuCfg.Index,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

func (h *AdminHandlers) AdminStopHandler(w http.ResponseWriter, r *http.Request) {
	active := h.runtime.ActiveInfo()

	err := h.runtime.StopActive()

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
	active := h.runtime.ActiveInfo()
	resp := adminLogsResponse{
		Running: active != nil,
		Logs:    h.runtime.ActiveLogs(),
	}
	if appLogPath := h.appLogPath(); appLogPath != "" {
		if _, err := os.Stat(appLogPath); err == nil {
			resp.AppLogOK = true
		}
	}

	if active != nil {
		resp.Name = active.Name
		resp.Ready = active.Ready
		resp.StartedAt = active.Started
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminHandlers) AdminAppLogsHandler(w http.ResponseWriter, r *http.Request) {
	appLogPath := h.appLogPath()
	if appLogPath == "" {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(appLogPath); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(appLogPath)))
	http.ServeFile(w, r, appLogPath)
}

func (h *AdminHandlers) appLogPath() string {
	if h.logger == nil {
		return ""
	}
	if provider, ok := h.logger.(logging.LogPathProvider); ok {
		return provider.LogPath()
	}
	return ""
}

func (h *AdminHandlers) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	resp := h.admin.MetricsSnapshot()
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
		h.admin.SetModelDir(req.ModelDir)
	}
	if req.LlamaBinary != "" {
		h.runtime.SetBinary(req.LlamaBinary)
	}
	if req.ModelHost != "" {
		h.runtime.SetModelHost(req.ModelHost)
	}
	gpuCfg := h.admin.GPUConfig()
	if req.GPUProvider != "" {
		gpuCfg.Provider = req.GPUProvider
	}
	if req.GPUBinary != "" {
		gpuCfg.Binary = req.GPUBinary
	}
	if req.GPUIndex != nil {
		gpuCfg.Index = *req.GPUIndex
	}
	h.admin.SetGPUConfig(gpuCfg)

	if err := h.admin.UpdateConfig(func(cfg *models.Config) {
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

	h.admin.RefreshMetricsService()

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

	fullPath := h.admin.ResolveModelPath(filename, req.Path)
	if _, err := os.Stat(fullPath); err != nil {
		writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
		return
	}

	if req.Port == 0 {
		active := h.runtime.ActiveInfo()
		activePort := 0
		if active != nil {
			activePort = active.Port
		}
		req.Port = nextAvailablePort(h.runtime.ListModels(), activePort)
	}

	args := req.Args
	if defaults := h.admin.DefaultArgs(); len(defaults) > 0 {
		args = append(append([]string{}, defaults...), req.Args...)
	}

	runtimeCfg := models.ModelConfig{
		Name:     req.Name,
		Filename: filename,
		Path:     fullPath,
		Args:     args,
		Port:     req.Port,
	}

	if err := h.runtime.AddModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrModelExists) {
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

	if err := h.admin.PersistModel(persistCfg); err != nil {
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

	var existing models.ModelConfig
	found := false
	for _, m := range h.runtime.ListModels() {
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
	fullPath := h.admin.ResolveModelPath(req.Filename, req.Path)
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

	if err := h.runtime.UpdateModel(runtimeCfg); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrUnknownModel) {
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

	if err := h.admin.PersistReplaceModel(persistCfg); err != nil {
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

	if err := h.runtime.RemoveModel(name); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, llm.ErrUnknownModel) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, "unable to delete model: "+err.Error())
		return
	}

	if err := h.admin.PersistDeleteModel(name); err != nil {
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

func (h *AdminHandlers) AdminPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Render the admin UI template with version info
	err := adminTmpl.Execute(w, AdminView{
		Version:   h.version,
		Commit:    h.commit,
		BuildDate: h.buildDate,
	})
	if err != nil {
		http.Error(w, "template render failed", http.StatusInternalServerError)
	}
}
