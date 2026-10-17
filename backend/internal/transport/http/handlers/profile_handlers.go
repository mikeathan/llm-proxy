package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"llm-proxy/internal/core/assistant/profile"
)

// ProfileCatalog is the part of the prompt-profile catalog the handlers use.
type ProfileCatalog interface {
	Get(id string) (profile.Profile, bool)
	List() []profile.Profile
}

// ProfileHandlers serves the list of prompt profiles a chat, workspace or automation can choose from.
type ProfileHandlers struct {
	catalog ProfileCatalog
}

func NewProfileHandlers(catalog ProfileCatalog) *ProfileHandlers {
	return &ProfileHandlers{catalog: catalog}
}

// profileItem is a profile as the picker needs it: what it is and what it is the default for, never its instructions.
type profileItem struct {
	profile.Profile
	DefaultFor []profile.Kind `json:"default_for"`
}

// List returns every profile, sorted by id.
func (h *ProfileHandlers) List(w http.ResponseWriter, _ *http.Request) {
	profiles := h.catalog.List()
	items := make([]profileItem, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, profileItem{Profile: p, DefaultFor: profile.DefaultFor(p.ID)})
	}
	respondJSON(w, items)
}

// validateProfileID accepts "" (the default) or the id of a profile the catalog knows; the error names the known ones.
// With no catalog wired there is nothing to check against.
func validateProfileID(catalog ProfileCatalog, field, id string) error {
	if id == "" || catalog == nil {
		return nil
	}
	if _, ok := catalog.Get(id); ok {
		return nil
	}
	known := make([]string, 0, 8)
	for _, p := range catalog.List() {
		known = append(known, p.ID)
	}
	return fmt.Errorf("unknown %s %q: choose one of %s (empty = the default)", field, id, strings.Join(known, ", "))
}
