package handlers

import (
	"net/http"

	"llm-proxy/models"
)

// MCPHandlers serves MCP server CRUD endpoints.
type MCPHandlers struct {
	admin AdminService
}

func NewMCPHandlers(admin AdminService) *MCPHandlers {
	return &MCPHandlers{admin: admin}
}

// AdminMCPListHandler handles GET /admin/api/mcp
func (h *MCPHandlers) AdminMCPListHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.ListMCPServers())
}

// AdminMCPAddHandler handles POST /admin/api/mcp
func (h *MCPHandlers) AdminMCPAddHandler(w http.ResponseWriter, r *http.Request) {
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

// AdminMCPUpdateHandler handles PUT /admin/api/mcp
func (h *MCPHandlers) AdminMCPUpdateHandler(w http.ResponseWriter, r *http.Request) {
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

// AdminMCPRemoveHandler handles DELETE /admin/api/mcp
func (h *MCPHandlers) AdminMCPRemoveHandler(w http.ResponseWriter, r *http.Request) {
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
