package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/models"
)

// ProfileCatalog is the part of the prompt-profile catalog the handlers use.
type ProfileCatalog interface {
	Get(id string) (profile.Profile, bool)
	List() []profile.Profile
	Save(p profile.Profile) error
	Delete(id string) error
	DelegatingTo(id string) []string
	HasBuiltin(id string) bool
}

// ProfileHandlers serves the prompt profiles a chat, workspace or automation can choose from, and lets the user write
// their own.
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

// profileDetail is one profile as the editor needs it: the list entry plus the instructions and who still names it.
type profileDetail struct {
	profileItem
	Instructions string   `json:"instructions"`
	DelegatingTo []string `json:"delegating_to"`
	// ReplacesBuiltin is true for the user's own file that has a built-in's id: deleting it brings the built-in back.
	ReplacesBuiltin bool `json:"replaces_builtin"`
}

// profileInput is what the editor sends; the id is in the path and the source is always the user's.
type profileInput struct {
	Name         string            `json:"name"`
	Summary      string            `json:"summary"`
	Instructions string            `json:"instructions"`
	AnswerFirst  bool              `json:"answer_first"`
	MayAsk       bool              `json:"may_ask"`
	MaxSteps     int               `json:"max_steps"`
	Tools        []string          `json:"tools"`
	Memory       models.MemoryMode `json:"memory"`
	MemorySave   profile.SaveMode  `json:"memory_save"`
	Reminders    bool              `json:"reminders"`
	Delegates    []string          `json:"delegates"`
}

func (in profileInput) profile(id string) profile.Profile {
	return profile.Profile{
		ID: id, Name: strings.TrimSpace(in.Name), Summary: strings.TrimSpace(in.Summary), Source: profile.User,
		Instructions: strings.TrimSpace(in.Instructions), AnswerFirst: in.AnswerFirst, MayAsk: in.MayAsk, MaxSteps: in.MaxSteps,
		Tools: in.Tools, Memory: in.Memory, MemorySave: in.MemorySave.Effective(), Reminders: in.Reminders, Delegates: in.Delegates,
	}
}

func (h *ProfileHandlers) detail(p profile.Profile) profileDetail {
	delegating := h.catalog.DelegatingTo(p.ID)
	if delegating == nil {
		delegating = []string{}
	}
	return profileDetail{
		profileItem: profileItem{Profile: p, DefaultFor: profile.DefaultFor(p.ID)}, Instructions: p.Instructions,
		DelegatingTo: delegating, ReplacesBuiltin: p.Source == profile.User && h.catalog.HasBuiltin(p.ID),
	}
}

// Schema returns the settings of a profile with their help text, the one table the parser's rules are written against.
func (h *ProfileHandlers) Schema(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, profile.Schema())
}

// Get returns one profile with its instructions.
func (h *ProfileHandlers) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := h.catalog.Get(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "no such profile")
		return
	}
	respondJSON(w, h.detail(p))
}

// Put creates or replaces one of the user's own profiles.
func (h *ProfileHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var in profileInput
	if !decodeJSONBody(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if err := h.catalog.Save(in.profile(id)); err != nil {
		writeProfileError(w, err)
		return
	}
	p, _ := h.catalog.Get(id)
	respondJSON(w, h.detail(p))
}

// profileDeleted says what a delete left behind: the built-in that came back, and the profiles that still name it.
type profileDeleted struct {
	RestoredBuiltin bool     `json:"restored_builtin"`
	DelegatingTo    []string `json:"delegating_to"`
}

// Delete removes one of the user's own profiles. Whatever still names it is not blocked: a run skips an id that is gone
// and the next level applies.
func (h *ProfileHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stillNamed := h.catalog.DelegatingTo(id)
	if stillNamed == nil {
		stillNamed = []string{}
	}
	if err := h.catalog.Delete(id); err != nil {
		writeProfileError(w, err)
		return
	}
	_, restored := h.catalog.Get(id)
	respondJSON(w, profileDeleted{RestoredBuiltin: restored, DelegatingTo: stillNamed})
}

// writeProfileError answers a refused save or delete: what was wrong, and which setting it is about.
func writeProfileError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, profile.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, profile.ErrBuiltin):
		status = http.StatusConflict
	case errors.Is(err, profile.ErrNotFound):
		status = http.StatusNotFound
	}
	field, message := profile.Explain(err)
	if status == http.StatusInternalServerError {
		message = "could not write the profile"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "field": field})
}
