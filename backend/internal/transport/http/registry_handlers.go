package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
	"os"
)

func (h *AdminHandlers) AdminAddModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleAddModel(w, r)
}

func (h *AdminHandlers) AdminUpdateModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateModel(w, r)
}

func (h *AdminHandlers) AdminDeleteModelHandler(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteModel(w, r)
}

func (h *AdminHandlers) getModelsView() []adminModelView {
	models := h.admin.Models()
	out := make([]adminModelView, 0, len(models))

	for _, m := range models {
		view := adminModelView{
			Name:         m.Name,
			Provider:     m.Provider,
			Filename:     m.Filename,
			ResolvedPath: h.admin.ResolveModelPath(m.Provider, m.Filename),
			Args:         m.Args,
			Port:         m.Port,
		}
		out = append(out, view)
	}
	return out
}

// AdminRegistryHandler handles GET /admin/api/registry
func (h *AdminHandlers) AdminRegistryHandler(w http.ResponseWriter, r *http.Request) {
	reg := h.admin.GetRegistry()
	view := adminRegistryView{
		Catalogue:  reg.Catalogue,
		Providers:  reg.Providers,
		MCPServers: reg.MCPServers,
	}
	respondJSON(w, view)
}

// AdminRegistryPutHandler handles PUT /admin/api/registry
func (h *AdminHandlers) AdminRegistryPutHandler(w http.ResponseWriter, r *http.Request) {
	var req adminRegistryView
	if !decodeJSONBody(w, r, &req) {
		return
	}

	err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		reg.Catalogue = req.Catalogue
		reg.Providers = req.Providers
		reg.MCPServers = req.MCPServers
	})

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update registry: "+err.Error())
		return
	}

	respondJSON(w, h.admin.GetRegistry())
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
	manifests := providers.GetRegistry().List()
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
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeJSONError(w, http.StatusBadRequest, "model file not found")
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

	var runtimeArgs []string
	if len(req.Args) == 0 {
		runtimeArgs = existing.Args
	} else {
		runtimeArgs = append([]string(nil), req.Args...)
	}

	fullPath := ""
	if req.Provider == "local" {
		fullPath = h.admin.ResolveModelPath(req.Filename, req.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeJSONError(w, http.StatusBadRequest, "model file not found")
			return
		}
	}

	runtimeCfg := models.ModelConfig{
		Name:           req.Name,
		Provider:       req.Provider,
		Filename:       req.Filename,
		Path:           fullPath,
		Args:           runtimeArgs,
		Port:           req.Port,
		Environment:    h.admin.Environment(),
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
