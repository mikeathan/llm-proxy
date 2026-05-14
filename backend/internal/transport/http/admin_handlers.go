package api

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"mime"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func init() {
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
}

//go:embed all:frontend_dist
var frontendFS embed.FS

// modelDiscoveryCache caches the result of a model directory scan.
// It is invalidated automatically when the directory mtime changes
// (i.e. a file was added or removed).
type modelDiscoveryCache struct {
	mu      sync.Mutex
	dir     string
	dirMod  time.Time
	results []adminAvailableModel
}

func (c *modelDiscoveryCache) get(dir string) ([]adminAvailableModel, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dir != dir {
		return nil, false
	}
	if info, err := os.Stat(dir); err != nil || !info.ModTime().Equal(c.dirMod) {
		return nil, false // directory changed or gone
	}
	return c.results, true
}

func (c *modelDiscoveryCache) set(dir string, results []adminAvailableModel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Stat(dir)
	if err != nil {
		return
	}
	c.dir = dir
	c.dirMod = info.ModTime()
	c.results = results
}

type AdminHandlers struct {
	runtime        RuntimeService
	admin          AdminService
	logger         logging.Logger
	buildInfo      *buildinfo.Info
	discoveryCache modelDiscoveryCache
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
	Name           string                 `json:"name"`
	Provider       string                 `json:"provider"`
	Filename       string                 `json:"filename"`
	ResolvedPath   string                 `json:"resolved_path"`
	Args           []string               `json:"args"`
	Port           int                    `json:"port"`
	Endpoint       string                 `json:"endpoint"`
	Active         bool                   `json:"active"`
	Ready          bool                   `json:"ready"`
	ProviderConfig *models.ProviderConfig `json:"provider_config,omitempty"`
	Metadata       *models.ModelMetadata  `json:"metadata,omitempty"`
	Prefill        bool                   `json:"prefill"`
	MaxSteps       int                    `json:"max_steps"`
	ContextBudget  int                    `json:"context_budget"`
	ToolCallFormat string                 `json:"tool_call_format"`
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
	Name         string               `json:"name"`
	Filename     string               `json:"filename"`
	ResolvedPath string               `json:"resolved_path"`
	SizeBytes    int64                `json:"size_bytes"`
	Metadata     models.ModelMetadata `json:"metadata"`
}

type adminStateResponse struct {
	Models    []adminModelView      `json:"models"`
	Available []adminAvailableModel `json:"available,omitempty"`
	NextPort  int                   `json:"next_port"`
	Active    *adminActiveModel     `json:"active,omitempty"`
	Config    adminConfigView       `json:"config"`
}

type adminConfigView struct {
	WorkspacesDir       string                       `json:"workspaces_dir"`
	ModelHost           string                       `json:"model_host"`
	IdleTimeoutSecs     int                          `json:"idle_timeout_seconds"`
	GPUProvider         string                       `json:"gpu_provider"`
	GPUBinary           string                       `json:"gpu_binary"`
	GPUIndex            int                          `json:"gpu_index"`
	DefaultArgs         []string                     `json:"default_args"`
	ServiceClientID     string                       `json:"service_client_id,omitempty"`
	ServiceClientSecret string                       `json:"service_client_secret,omitempty"`
	PrimaryModel        string                       `json:"primary_model"`
	FallbackModel       string                       `json:"fallback_model"`
	Providers           map[string]adminProviderView `json:"providers"`
	Guardrails          models.AgentGuardrailsConfig `json:"guardrails"`
	Communication       models.CommunicationConfig   `json:"communication"`
	Search              models.SearchConfig          `json:"search"`
}

type adminSystemView struct {
	Bind            string               `json:"bind"`
	ModelHost       string               `json:"model_host"`
	IdleTimeoutSecs int                  `json:"idle_timeout_seconds"`
	WorkspacesDir   string               `json:"workspaces_dir"`
	GPU             models.GPUConfig     `json:"gpu"`
	Environment     map[string]string    `json:"environment"`
	Local           models.LocalSettings `json:"local"`
}

type adminRegistryView struct {
	Catalogue  []models.ModelRegistryEntry     `json:"catalogue"`
	Providers  map[string]adminProviderView    `json:"providers"`
	MCPServers []models.MCPServerRegistryEntry `json:"mcp_servers"`
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
	if err := decodeJSON(w, r, v); err != nil {
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
	settings := h.admin.GetSettings()
	var available []adminAvailableModel
	if v := strings.ToLower(r.URL.Query().Get("available")); v == "1" || v == "true" {
		modelDir := settings.Local.ModelDir
		if cached, ok := h.discoveryCache.get(modelDir); ok {
			available = cached
		} else {
			available = discoverModelFiles(r.Context(), modelDir, modelsList)
			h.discoveryCache.set(modelDir, available)
		}
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
	sys := h.admin.GetSystem()
	reg := h.admin.GetRegistry()
	id, secret := h.admin.ServiceCredentials()
	state := adminStateResponse{
		Models:    h.getModelsView(r.Context(), modelsList, activeName, activeDetails != nil && activeDetails.Ready),
		Available: available,
		NextPort:  nextPort,
		Active:    activeDetails,
		Config: adminConfigView{
			WorkspacesDir:       sys.WorkspacesDir,
			ModelHost:           sys.Server.ModelHost,
			IdleTimeoutSecs:     sys.Server.IdleTimeoutSecs,
			GPUProvider:         h.admin.GPUConfig().Provider,
			GPUBinary:           h.admin.GPUConfig().Binary,
			GPUIndex:            h.admin.GPUConfig().Index,
			DefaultArgs:         settings.Local.DefaultArgs,
			ServiceClientID:     id,
			ServiceClientSecret: secret,
			PrimaryModel:        reg.PrimaryModel,
			FallbackModel:       reg.FallbackModel,
			Providers:           h.getProvidersView(),
			Guardrails:          h.admin.GetGuardrails(),
			Communication:       reg.Communication,
			Search:              reg.Search,
		},
	}

	respondJSON(w, state)
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

	if _, err := fs.Stat(fsys, p); os.IsNotExist(err) {
		p = "index.html"
	}

	if p == "index.html" {
		r.URL.Path = "/"
	} else {
		r.URL.Path = "/" + p
	}

	http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
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
