package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/process"
	"llm-proxy/models"
)

// ProcessHandlers serves model lifecycle, log, metrics, and process endpoints.
type ProcessHandlers struct {
	runtime RuntimeService
	admin   AdminService
	logger  logging.Logger
}

// logTailBytes bounds the bytes served for any log endpoint so a growing log
// file never turns a 10s frontend poll into a full-file read + transfer (ops
// review 2026-08-28 finding #3).
const logTailBytes = 64 * 1024

// readTail returns at most the last maxBytes of f, or the whole file when it is
// smaller. Shared by the app-log tail and workspace process-log endpoints.
func readTail(f *os.File, maxBytes int64) ([]byte, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maxBytes {
		if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(f)
}

func NewProcessHandlers(runtime RuntimeService, admin AdminService, logger logging.Logger) *ProcessHandlers {
	return &ProcessHandlers{runtime: runtime, admin: admin, logger: logger}
}

func (h *ProcessHandlers) AdminStartHandler(w http.ResponseWriter, r *http.Request) {
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
		respondJSON(w, adminStartResponse{Status: models.ModelStatusStarting, Model: req.Name})
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "model error: "+err.Error())
		return
	}

	resp := adminStartResponse{
		Status:   "started",
		Model:    req.Name,
		Endpoint: network.FormatURL(mi.Host, mi.Port),
		Port:     mi.Port,
	}

	respondJSON(w, resp)
}

func (h *ProcessHandlers) AdminStopHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *ProcessHandlers) AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	active := h.runtime.ActiveInfo()
	resp := adminLogsResponse{
		Running: active != nil,
		Logs:    h.runtime.ActiveLogs(),
		Error:   h.runtime.LastModelError(),
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

func (h *ProcessHandlers) AdminLogsClearHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.ClearLogs(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to clear logs: "+err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (h *ProcessHandlers) AdminLogLevelHandler(w http.ResponseWriter, r *http.Request) {
	if h.logger == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "logger unavailable")
		return
	}
	resp := adminLogLevelResponse{Level: string(h.logger.Level())}
	respondJSON(w, resp)
}

func (h *ProcessHandlers) AdminLogLevelUpdateHandler(w http.ResponseWriter, r *http.Request) {
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

	h.admin.UpdateSystem(func(sys *models.SystemConfig) {
		sys.Server.LogLevel = string(level)
	})

	resp := adminLogLevelResponse{Level: string(level)}
	respondJSON(w, resp)
}

func (h *ProcessHandlers) AdminAppLogsHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *ProcessHandlers) AdminAppLogsTailHandler(w http.ResponseWriter, r *http.Request) {
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

	b, err := readTail(f, logTailBytes)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read log: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"logs": string(b), "running": "true"})
}

func (h *ProcessHandlers) AdminAppLogsClearHandler(w http.ResponseWriter, r *http.Request) {
	appLogPath := h.appLogPath()
	if appLogPath == "" {
		writeJSONError(w, http.StatusNotFound, "log path unknown")
		return
	}
	if err := os.Truncate(appLogPath, 0); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to truncate log: "+err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (h *ProcessHandlers) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	resp := h.admin.MetricsSnapshot()
	respondJSON(w, resp)
}

func (h *ProcessHandlers) AdminWorkspaceProcessLogsHandler(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	workspaceID := vals[0]

	logger := h.admin.ProcessLogger(workspaceID)
	lp, ok := logger.(interface{ LogPath() string })
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "logger does not support file reading")
		return
	}

	path := lp.LogPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.Write([]byte(""))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to read process log: "+err.Error())
		return
	}
	defer f.Close()

	data, err := readTail(f, logTailBytes)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read process log: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (h *ProcessHandlers) AdminProcessesHandler(w http.ResponseWriter, r *http.Request) {
	activeInfo := h.runtime.ActiveInfo()
	activePID := 0
	if activeInfo != nil {
		activePID = activeInfo.PID
	}

	processes, err := process.ListByBinary("llama-server", activePID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if processes == nil {
		processes = []process.Info{}
	}

	respondJSON(w, map[string]any{"processes": processes})
}

func (h *ProcessHandlers) AdminProcessKillHandler(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pid")
		return
	}

	activeInfo := h.runtime.ActiveInfo()
	if activeInfo != nil && activeInfo.PID == pid {
		if err := h.runtime.StopActive(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, map[string]any{"status": "stopped", "pid": pid})
		return
	}

	if err := process.Kill(pid); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]any{"status": "stopped", "pid": pid})
}

func (h *ProcessHandlers) appLogPath() string {
	if h.logger == nil {
		return ""
	}
	if provider, ok := h.logger.(logging.LogPathProvider); ok {
		return provider.LogPath()
	}
	return ""
}
