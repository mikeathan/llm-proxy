package api

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func init() {
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
}

//go:embed all:frontend_dist
var frontendFS embed.FS

type AdminHandlers struct {
	runtime   RuntimeService
	admin     AdminService
	logger    logging.Logger
	buildInfo *buildinfo.Info
}

func NewAdminHandlers(
	runtime RuntimeService,
	admin AdminService,
	logger logging.Logger,
	buildInfo *buildinfo.Info,
) *AdminHandlers {
	return &AdminHandlers{
		runtime:   runtime,
		admin:     admin,
		logger:    logger,
		buildInfo: buildInfo,
	}
}

type adminModelView struct {
	Name           string                `json:"name"`
	Provider       string                `json:"provider"`
	Filename       string                `json:"filename"`
	ResolvedPath   string                `json:"resolved_path"`
	Args           []string              `json:"args"`
	Port           int                   `json:"port"`
	Endpoint       string                `json:"endpoint"`
	Active         bool                  `json:"active"`
	Ready          bool                  `json:"ready"`
	ProviderConfig models.ProviderConfig `json:"provider_config,omitempty"`
}

type adminActiveModel struct {
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
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
	ModelDir            string                         `json:"model_dir"`
	WorkspacesDir       string                         `json:"workspaces_dir"`
	GPU                 models.GPUConfig               `json:"gpu"`
	Binary              string                         `json:"binary"`
	IdleTimeout         int                            `json:"idle_timeout_seconds"`
	ServiceClientID     string                         `json:"service_client_id,omitempty"`
	ServiceClientSecret string                         `json:"service_client_secret,omitempty"`
	DefaultArgs         []string                       `json:"default_args"`
	PrimaryModel        string                         `json:"primary_model"`
	FallbackModel       string                         `json:"fallback_model"`
	Providers           map[string]adminProviderView   `json:"providers"`
	Guardrails          models.AgentGuardrailsConfig   `json:"guardrails"`
	Communication       models.CommunicationConfig     `json:"communication"`
	Search              models.SearchConfig            `json:"search"`
}

type adminProviderView struct {
	models.ProviderItem
	APIKeys []models.APIKeyItem `json:"api_keys,omitempty"`
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

type adminLogLevelResponse struct {
	Level string `json:"level"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	respondJSON(w, map[string]string{"error": msg})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := decodeJSON(r, v); err != nil {
		if errors.Is(err, ErrUnsupportedContentType) {
			writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		} else {
			writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		}
		return false
	}
	return true
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
			Provider:  ai.Provider,
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
	state := adminStateResponse{
		Models:    make([]adminModelView, 0, len(modelsList)),
		Available: available,
		NextPort:  nextPort,
		Active:    activeDetails,
		Config: adminConfigView{
			ModelDir:      h.admin.ModelDir(),
			WorkspacesDir: h.admin.WorkspacesDir(),
			GPU:           h.admin.GPUConfig(),
			Binary:        h.admin.CurrentBinary(),
			IdleTimeout:   h.admin.CurrentIdleTimeout(),
			DefaultArgs:   h.admin.DefaultArgs(),
			PrimaryModel:  h.admin.Config().Server.PrimaryModel,
			FallbackModel: h.admin.Config().Server.FallbackModel,
			Providers:     h.getProvidersView(),
			Guardrails:    tools.GetDefaultGuardrails(h.admin.RootDir()),
			Communication: h.admin.Config().Communication,
			Search:        h.admin.Config().Search,
		},
	}

	rawModels := h.admin.Models()
	rawArgs := map[string][]string{}
	for _, raw := range rawModels {
		rawArgs[raw.Name] = raw.Args
	}

	for _, mc := range modelsList {
		filename := mc.Filename
		if filename == "" && mc.Path != "" {
			filename = filepath.Base(mc.Path)
		}

		// Use raw arguments from the config, not the runtime combinations which inject DefaultArgs.
		args := rawArgs[mc.Name]
		if args == nil {
			args = mc.Args
		}

		state.Models = append(state.Models, adminModelView{
			Name:           mc.Name,
			Provider:       mc.Provider,
			Filename:       filename,
			ResolvedPath:   mc.Path,
			Args:           args,
			Port:           mc.Port,
			Endpoint:       fmt.Sprintf("http://%s:%d", host, mc.Port),
			Active:         mc.Name == activeName,
			Ready:          mc.Name == activeName && activeDetails != nil && activeDetails.Ready,
			ProviderConfig: mc.ProviderConfig,
		})
	}

	respondJSON(w, state)
}

func (h *AdminHandlers) AdminStartHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		if !decodeJSONBody(w, r, &req) {
			return
		}
	}
	if req.Name == "" {
		req.Name = r.URL.Query().Get("name")
	}

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model name")
		return
	}

	mi, err := h.runtime.EnsureModel(r.Context(), req.Name)
	if err == models.ErrModelStarting {
		w.WriteHeader(http.StatusAccepted)
		respondJSON(w, adminStartResponse{Status: "starting", Model: req.Name})
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

	respondJSON(w, resp)
}

func (h *AdminHandlers) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleAddModel(w, r)
}
func (h *AdminHandlers) getProvidersView() map[string]adminProviderView {
	providers := h.admin.Providers()
	view := make(map[string]adminProviderView, len(providers))

	for id, p := range providers {
		view[id] = adminProviderView{
			ProviderItem: p,
			APIKeys:      h.admin.Secrets().MaskedProviderKeys(id),
		}
	}
	return view
}

func (h *AdminHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	cfg := adminConfigView{
		ModelDir:      h.admin.ModelDir(),
		WorkspacesDir: h.admin.WorkspacesDir(),
		GPU:           h.admin.GPUConfig(),
		Binary:        h.admin.CurrentBinary(),
		IdleTimeout:   h.admin.CurrentIdleTimeout(),
		DefaultArgs:   h.admin.DefaultArgs(),
		PrimaryModel:  h.admin.Config().Server.PrimaryModel,
		FallbackModel: h.admin.Config().Server.FallbackModel,
		Providers:     h.getProvidersView(),
		Guardrails:    tools.GetDefaultGuardrails(h.admin.RootDir()),
		Communication: h.admin.Config().Communication,
		Search:        h.admin.Config().Search,
	}
	respondJSON(w, cfg)
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

	respondJSON(w, resp)
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

	respondJSON(w, resp)
}

func (h *AdminHandlers) AdminLogsClearHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.ClearLogs(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to clear logs: "+err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (h *AdminHandlers) AdminLogLevelHandler(w http.ResponseWriter, r *http.Request) {
	if h.logger == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "logger unavailable")
		return
	}
	resp := adminLogLevelResponse{Level: string(h.logger.Level())}
	respondJSON(w, resp)
}

func (h *AdminHandlers) AdminLogLevelUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if h.logger == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "logger unavailable")
		return
	}
	var req struct {
		Level string `json:"level"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	level, err := parseLogLevel(req.Level)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.logger.Info("log level updated", "level", level)

	h.logger.SetLevel(level)
	resp := adminLogLevelResponse{Level: string(level)}
	respondJSON(w, resp)
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

func (h *AdminHandlers) AdminAppLogsTailHandler(w http.ResponseWriter, r *http.Request) {
	appLogPath := h.appLogPath()
	if appLogPath == "" {
		writeJSONError(w, http.StatusNotFound, "log path unknown")
		return
	}
	f, err := os.Open(appLogPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondJSON(w, map[string]string{"logs": "Log file does not exist yet.", "running": "false"})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to open log: "+err.Error())
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to stat log: "+err.Error())
		return
	}

	const tailSize = 64 * 1024
	if stat.Size() > tailSize {
		if _, err := f.Seek(-tailSize, io.SeekEnd); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to seek log: "+err.Error())
			return
		}
	}

	b, err := io.ReadAll(f)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read log: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"logs": string(b), "running": "true"})
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

func (h *AdminHandlers) AdminAppLogsClearHandler(w http.ResponseWriter, r *http.Request) {
	appLogPath := h.appLogPath()
	if appLogPath == "" {
		writeJSONError(w, http.StatusNotFound, "log path unknown")
		return
	}
	// Truncate the file to 0 size.
	// We might have an open handle in the logger, but truncating should work on most OSs.
	if err := os.Truncate(appLogPath, 0); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to truncate log: "+err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func parseLogLevel(input string) (logging.Level, error) {
	level := strings.ToUpper(strings.TrimSpace(input))
	switch level {
	case string(logging.LevelDebug):
		return logging.LevelDebug, nil
	case string(logging.LevelInfo):
		return logging.LevelInfo, nil
	case string(logging.LevelWarn):
		return logging.LevelWarn, nil
	case string(logging.LevelError):
		return logging.LevelError, nil
	default:
		return "", fmt.Errorf("invalid log level: %s", input)
	}
}

func (h *AdminHandlers) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	resp := h.admin.MetricsSnapshot()
	respondJSON(w, resp)
}

func (h *AdminHandlers) AdminUpdateModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateModel(w, r)
}

func (h *AdminHandlers) AdminDeleteModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteModel(w, r)
}

func (h *AdminHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspacesDir       string                         `json:"workspaces_dir"`
		ModelHost           string                         `json:"model_host"`
		GPUProvider         string                         `json:"gpu_provider"`
		GPUBinary           string                         `json:"gpu_binary"`
		GPUIndex            *int                           `json:"gpu_index"`
		ServiceClientID     string                         `json:"service_client_id"`
		ServiceClientSecret string                         `json:"service_client_secret"`
		Environment         map[string]string              `json:"environment"`
		DefaultArgs         []string                       `json:"default_args"`
		PrimaryModel        string                         `json:"primary_model"`
		FallbackModel       string                         `json:"fallback_model"`
		Providers           map[string]models.ProviderItem `json:"providers"`
		Guardrails          *models.AgentGuardrailsConfig  `json:"guardrails,omitempty"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.WorkspacesDir != "" {
		h.admin.SetWorkspacesDir(req.WorkspacesDir)
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

	envUpdates := map[string]string{}
	if req.ServiceClientID != "" {
		os.Setenv("SERVICE_CLIENT_ID", req.ServiceClientID)
		envUpdates["SERVICE_CLIENT_ID"] = req.ServiceClientID
	}
	if req.ServiceClientSecret != "" {
		os.Setenv("SERVICE_CLIENT_SECRET", req.ServiceClientSecret)
		envUpdates["SERVICE_CLIENT_SECRET"] = req.ServiceClientSecret
	}
	// Update .env file
	if len(envUpdates) > 0 {
		envPath, _ := config.EnvFilePaths() // Using new config package
		// We only write to the main .env for now or checking existence?
		// Logic was: utils.UpdateEnvFile(envPath, envUpdates)
		// Assuming envPath returned by EnvFilePaths (first return ref) is target.

		if err := config.UpdateEnvFile(envPath, envUpdates); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save env: "+err.Error())
			return
		}
	}

	if err := h.admin.UpdateConfig(func(cfg *models.Config) {
		if req.WorkspacesDir != "" {
			cfg.WorkspacesDir = req.WorkspacesDir
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
		if req.Environment != nil {
			cfg.Server.Environment = req.Environment
		}
		if req.DefaultArgs != nil {
			cfg.Server.DefaultArgs = req.DefaultArgs
		}
		if req.PrimaryModel != "" {
			cfg.Server.PrimaryModel = req.PrimaryModel
		}
		if req.FallbackModel != "" {
			cfg.Server.FallbackModel = req.FallbackModel
		}
		if req.Providers != nil {
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]models.ProviderItem)
			}
			for k, v := range req.Providers {
				cfg.Providers[k] = v
			}
		}

		// Guardrails are no longer stored in config.json.
		// We explicitly nil it out to ensure it's removed from disk if it existed,
		// as it's now handled by SyncGuardrails (manifest files).
		cfg.Guardrails = nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	if req.Environment != nil {
		for _, m := range h.runtime.ListModels() {
			m.Environment = req.Environment
			_ = h.runtime.UpdateModel(m)
		}
	}
	if req.Guardrails != nil {
		if err := h.admin.SyncGuardrails(*req.Guardrails); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to sync guardrails: "+err.Error())
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// MCP Handlers

func (h *AdminHandlers) AdminMCPListHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.ListMCPServers())
}

func (h *AdminHandlers) AdminMCPAddHandler(w http.ResponseWriter, r *http.Request) {
	var req models.MCPServerConfig
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name == "" || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	if err := h.admin.AddMCPServer(req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to add mcp server: "+err.Error())
		return
	}

	respondJSON(w, req)
}

func (h *AdminHandlers) AdminMCPUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.MCPServerConfig
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.admin.UpdateMCPServer(req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update mcp server: "+err.Error())
		return
	}

	respondJSON(w, req)
}

func (h *AdminHandlers) AdminMCPRemoveHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing name")
		return
	}

	if err := h.admin.RemoveMCPServer(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to remove mcp server: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandlers) AdminListProviderManifestsHandler(w http.ResponseWriter, r *http.Request) {
	manifests := llm.GetRegistry().List()
	respondJSON(w, manifests)
}

func (h *AdminHandlers) AdminListProviderModelsHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	apiKeyName := r.URL.Query().Get("api_key_name")
	models, err := h.runtime.ListProviderModels(r.Context(), provider, apiKeyName)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list models: "+err.Error())
		return
	}

	respondJSON(w, models)
}

func (h *AdminHandlers) AdminTestProviderConnectionHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	// api_key is optional: when supplied by the caller it overrides the saved config,
	// allowing the user to test a key before saving it.
	apiKey := r.URL.Query().Get("api_key")
	apiKeyName := r.URL.Query().Get("api_key_name")

	err := h.runtime.TestProviderConnection(r.Context(), provider, apiKey, apiKeyName)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "connection test failed: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok", "message": "Connection successful"})
}

func (h *AdminHandlers) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string                `json:"name"`
		Provider       string                `json:"provider"`
		Filename       string                `json:"filename"`
		ModelID        string                `json:"model_id"`
		Path           string                `json:"path"`
		Args           []string              `json:"args"`
		Port           int                   `json:"port"`
		ProviderConfig models.ProviderConfig `json:"provider_config"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Provider == "" {
		req.Provider = "local"
	}

	filename := strings.TrimSpace(req.Filename)
	if filename == "" && req.Path != "" {
		filename = filepath.Base(req.Path)
	}
	// For cloud providers, model_id serves as the identifier
	if filename == "" && req.ModelID != "" {
		filename = strings.TrimSpace(req.ModelID)
	}
	if filename == "" {
		writeJSONError(w, http.StatusBadRequest, "missing model identifier (filename or model_id)")
		return
	}

	if req.Name == "" {
		ext := filepath.Ext(filename)
		req.Name = strings.TrimSuffix(filename, ext)
	}

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(filename, req.Path)
		if _, err := os.Stat(fullPath); err != nil {
			writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
			return
		}
	}

	if req.Port == 0 && req.Provider == "local" {
		active := h.runtime.ActiveInfo()
		activePort := 0
		if active != nil {
			activePort = active.Port
		}
		req.Port = nextAvailablePort(h.runtime.ListModels(), activePort)
	}

	// Use default args if user-provided args are empty
	var runtimeArgs []string
	if len(req.Args) == 0 {
		runtimeArgs = append([]string(nil), h.admin.DefaultArgs()...)
	} else {
		runtimeArgs = append([]string(nil), req.Args...)
	}

	runtimeCfg := models.ModelConfig{
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       filename,
		Path:           fullPath,
		Args:           runtimeArgs,
		Port:           req.Port,
		Environment:    h.admin.Environment(),
		ProviderConfig: req.ProviderConfig,
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
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       filename,
		Args:           append([]string{}, req.Args...),
		Port:           req.Port,
		ProviderConfig: req.ProviderConfig,
	}

	if err := h.admin.PersistModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "saved model but failed to persist config: "+err.Error())
		return
	}

	respondJSON(w, runtimeCfg)
}

func (h *AdminHandlers) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string                `json:"name"`
		Provider       string                `json:"provider"`
		Filename       string                `json:"filename"`
		ModelID        string                `json:"model_id"`
		Path           string                `json:"path"`
		Args           []string              `json:"args"`
		Port           int                   `json:"port"`
		ProviderConfig models.ProviderConfig `json:"provider_config"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

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

	if req.Provider == "" {
		req.Provider = existing.Provider
	}
	if req.Provider == "" {
		req.Provider = "local"
	}
	if req.Filename == "" && req.Path != "" {
		req.Filename = filepath.Base(req.Path)
	}
	if req.Filename == "" && req.ModelID != "" {
		req.Filename = strings.TrimSpace(req.ModelID)
	}
	if req.Filename == "" {
		req.Filename = existing.Filename
	}
	if req.Port == 0 && req.Provider == "local" {
		req.Port = existing.Port
	}
	if len(req.Args) == 0 {
		req.Args = existing.Args
	}

	// Use default args if user-provided args are empty
	var runtimeArgs []string
	if len(req.Args) == 0 {
		runtimeArgs = append([]string(nil), h.admin.DefaultArgs()...)
	} else {
		runtimeArgs = append([]string(nil), req.Args...)
	}

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(req.Filename, req.Path)
		if _, err := os.Stat(fullPath); err != nil {
			writeJSONError(w, http.StatusBadRequest, "model file not found: "+err.Error())
			return
		}
	}

	env := make(map[string]string)
	for k, v := range h.admin.Environment() {
		env[k] = v
	}

	for _, raw := range h.admin.Models() {
		if raw.Name == req.Name {
			for k, v := range raw.Environment {
				env[k] = v
			}
			break
		}
	}

	runtimeCfg := models.ModelConfig{
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       req.Filename,
		Path:           fullPath,
		Args:           runtimeArgs,
		Port:           req.Port,
		Environment:    env,
		ProviderConfig: req.ProviderConfig,
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
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       req.Filename,
		Args:           append([]string{}, req.Args...),
		Port:           req.Port,
		ProviderConfig: req.ProviderConfig,
	}

	if err := h.admin.PersistReplaceModel(persistCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "updated model but failed to persist config: "+err.Error())
		return
	}

	respondJSON(w, runtimeCfg)
}

func (h *AdminHandlers) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		var req struct {
			Name string `json:"name"`
		}
		if r.Header.Get("Content-Type") == "application/json" {
			if !decodeJSONBody(w, r, &req) {
				return
			}
		}
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
	fsys, err := fs.Sub(frontendFS, "frontend_dist")
	if err != nil {
		http.Error(w, "Failed to load UI assets", http.StatusInternalServerError)
		return
	}

	p := strings.TrimPrefix(r.URL.Path, "/admin")
	if p == "" || p == "/" {
		p = "index.html"
	} else {
		p = strings.TrimPrefix(p, "/")
	}

	// Check if file exists, if not serve index.html (SPA)
	if _, err := fs.Stat(fsys, p); os.IsNotExist(err) {
		p = "index.html"
	}

	// If we serve index.html, we must reset the URL path because FileServer looks at it
	if p == "index.html" {
		r.URL.Path = "/"
	} else {
		r.URL.Path = "/" + p
	}

	http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
}
func (h *AdminHandlers) AdminWorkspaceProcessLogsHandler(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}

	logger := h.admin.ProcessLogger(workspaceID)
	lp, ok := logger.(interface{ LogPath() string })
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "logger does not support file reading")
		return
	}

	path := lp.LogPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.Write([]byte(""))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to read process log: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}
