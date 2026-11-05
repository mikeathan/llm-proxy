package handlers

import (
	"context"
	"net/http"
	"time"

	"llm-proxy/internal/core/netaudit/nuclei"
)

// downloadLimit bounds the operator's download of the reviewed web-check templates (24 small files).
const downloadLimit = 2 * time.Minute

// WebTemplates is what the web checks keep on disk and say about themselves: the reviewed templates and nuclei.
type WebTemplates interface {
	Ready() (bool, string)
	Status() nuclei.Status
	Download(ctx context.Context) error
}

// NetauditWebTemplatesHandlers serve the status of the reviewed web-check templates and the operator's request to download them.
// Nothing downloads them but this request (Constitution II.3).
type NetauditWebTemplatesHandlers struct{ templates WebTemplates }

func NewNetauditWebTemplatesHandlers(t WebTemplates) *NetauditWebTemplatesHandlers {
	return &NetauditWebTemplatesHandlers{templates: t}
}

type webTemplatesView struct {
	nuclei.Status
	Tag    string `json:"tag"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

func (h *NetauditWebTemplatesHandlers) view() webTemplatesView {
	ready, why := h.templates.Ready()
	return webTemplatesView{Status: h.templates.Status(), Tag: nuclei.ReviewedTag, Ready: ready, Reason: why}
}

// Get answers how many of the reviewed templates are present, whether the checks can run, and why not.
func (h *NetauditWebTemplatesHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.view())
}

// Download fetches the reviewed templates now and answers with the new status. A template that is not the reviewed file is not kept.
func (h *NetauditWebTemplatesHandlers) Download(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), downloadLimit)
	defer cancel()
	if err := h.templates.Download(ctx); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, h.view())
}
