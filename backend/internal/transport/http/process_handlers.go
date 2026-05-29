package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/process"
	"llm-proxy/models"
)

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

	h.admin.UpdateSystem(func(sys *models.SystemConfig) {
		sys.Server.LogLevel = string(level)
	})

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

func (h *AdminHandlers) AdminAppLogsClearHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandlers) AdminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	resp := h.admin.MetricsSnapshot()
	respondJSON(w, resp)
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

func (h *AdminHandlers) AdminProcessesHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandlers) AdminProcessKillHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandlers) appLogPath() string {
	if h.logger == nil {
		return ""
	}
	if provider, ok := h.logger.(logging.LogPathProvider); ok {
		return provider.LogPath()
	}
	return ""
}
