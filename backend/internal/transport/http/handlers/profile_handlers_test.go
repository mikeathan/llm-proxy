package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/platform/logging"
)

func builtinProfiles(t *testing.T) *profile.Catalog {
	t.Helper()
	return profile.MustCatalog(t.TempDir(), logging.NewNopLogger())
}

func TestProfileHandlers_ListNamesTheDefaultsAndHidesTheInstructions(t *testing.T) {
	h := NewProfileHandlers(builtinProfiles(t))
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/admin/api/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var items []struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Summary    string   `json:"summary"`
		Source     string   `json:"source"`
		DefaultFor []string `json:"default_for"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("got %d profiles, want the six built-ins", len(items))
	}
	byID := map[string]string{}
	for _, it := range items {
		byID[it.ID] = strings.Join(it.DefaultFor, ",")
		if it.Name == "" || it.Summary == "" || it.Source != "builtin" {
			t.Errorf("profile %s is incomplete: %+v", it.ID, it)
		}
	}
	if byID["general"] != "assistant" || byID["task-runner"] != "automation" {
		t.Errorf("defaults = %v, want general for assistant and task-runner for automation", byID)
	}
	if strings.Contains(rec.Body.String(), `"default_for":null`) {
		t.Error("default_for is an empty list, not null, for a profile that is no kind's default")
	}
	if strings.Contains(rec.Body.String(), "capable assistant") {
		t.Error("the instructions text is not part of the list")
	}
}

func TestValidateProfileID(t *testing.T) {
	catalog := builtinProfiles(t)
	if err := validateProfileID(catalog, "profile", ""); err != nil {
		t.Errorf("empty means the default: %v", err)
	}
	if err := validateProfileID(catalog, "profile", "research"); err != nil {
		t.Errorf("a built-in is valid: %v", err)
	}
	err := validateProfileID(catalog, "profile", "ghost")
	if err == nil || !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), "general") {
		t.Errorf("an unknown id is refused and the known ones are named: %v", err)
	}
	if err := validateProfileID(nil, "profile", "ghost"); err != nil {
		t.Errorf("with no catalog wired there is nothing to check against: %v", err)
	}
}
