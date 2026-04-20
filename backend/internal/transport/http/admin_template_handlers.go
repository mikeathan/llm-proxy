package api

import (
	"net/http"
	"strings"
)

// ListTemplatesHandler returns a list of all available task templates.
func (h *AdminHandlers) ListTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := h.admin.ListTemplates()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to list templates: "+err.Error())
		return
	}
	respondJSON(w, list)
}

// GetTemplateHandler returns the full details and content of a specific template.
func (h *AdminHandlers) GetTemplateHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/api/templates/")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "template id is required")
		return
	}

	template, err := h.admin.GetTemplate(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "Template not found: "+err.Error())
		return
	}
	respondJSON(w, template)
}
