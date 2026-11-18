package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if len(items) != 8 {
		t.Fatalf("got %d profiles, want the eight built-ins", len(items))
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

// serveProfile runs one editor request through the handlers with the path id set, as the router does.
func serveProfile(h *ProfileHandlers, handler func(http.ResponseWriter, *http.Request), method, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/admin/api/profiles/id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestProfileHandlers_EditorRoundTrip(t *testing.T) {
	h := NewProfileHandlers(builtinProfiles(t))
	rec := serveProfile(h, h.Put, http.MethodPut, "scout", `{"name":"Scout","summary":"Quick looks","instructions":"Look fast.","delegates":["web-researcher"],"tools":["search"],"max_steps":8}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}
	rec = serveProfile(h, h.Get, http.MethodGet, "scout", "")
	var got struct {
		ID, Name, Source, Instructions string
		MaxSteps                       int `json:"max_steps"`
		Delegates                      []string
		DelegatingTo                   []string `json:"delegating_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "user" || got.Instructions != "Look fast." || got.MaxSteps != 8 || len(got.Delegates) != 1 || got.DelegatingTo == nil {
		t.Errorf("GET after PUT = %+v (%s)", got, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"replaces_builtin":false`) {
		t.Errorf("a new profile replaces nothing: %s", rec.Body)
	}
	if rec = serveProfile(h, h.Get, http.MethodGet, "general", ""); !strings.Contains(rec.Body.String(), `"instructions":"`) {
		t.Errorf("a built-in's instructions are readable: %s", rec.Body)
	}
	if rec = serveProfile(h, h.Get, http.MethodGet, "ghost", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id = %d", rec.Code)
	}
}

func TestProfileHandlers_PutRefusals(t *testing.T) {
	h := NewProfileHandlers(builtinProfiles(t))
	for _, tc := range []struct {
		name, id, body, field string
		status                int
	}{
		{"missing name", "x", `{"instructions":"hi"}`, "name", http.StatusBadRequest},
		{"unknown helper", "x", `{"name":"X","instructions":"hi","delegates":["ghost"]}`, "delegates", http.StatusBadRequest},
		{"bad id", "Bad Id", `{"name":"X","instructions":"hi"}`, "", http.StatusBadRequest},
		{"a built-in's id", "research", `{"name":"X","instructions":"hi"}`, "", http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveProfile(h, h.Put, http.MethodPut, tc.id, tc.body)
			var out map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			if rec.Code != tc.status || out["field"] != tc.field || out["error"] == "" {
				t.Errorf("PUT = %d %v, want %d field %q", rec.Code, out, tc.status, tc.field)
			}
			if strings.Contains(out["error"], "invalid profile") {
				t.Errorf("the message keeps its internal prefix: %q", out["error"])
			}
		})
	}
	rec := serveProfile(h, h.Put, http.MethodPut, "x", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d", rec.Code)
	}
}

func TestProfileHandlers_DeleteSaysWhatStillNamesIt(t *testing.T) {
	h := NewProfileHandlers(builtinProfiles(t))
	serveProfile(h, h.Put, http.MethodPut, "helper", `{"name":"H","instructions":"help"}`)
	serveProfile(h, h.Put, http.MethodPut, "boss", `{"name":"B","instructions":"lead","delegates":["helper"]}`)
	rec := serveProfile(h, h.Delete, http.MethodDelete, "helper", "")
	var out struct {
		RestoredBuiltin bool     `json:"restored_builtin"`
		DelegatingTo    []string `json:"delegating_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("DELETE = %d %s", rec.Code, rec.Body)
	}
	if out.RestoredBuiltin || len(out.DelegatingTo) != 1 || out.DelegatingTo[0] != "boss" {
		t.Errorf("delete result = %+v", out)
	}
	if rec = serveProfile(h, h.Delete, http.MethodDelete, "general", ""); rec.Code != http.StatusConflict {
		t.Errorf("deleting a built-in = %d", rec.Code)
	}
	if rec = serveProfile(h, h.Delete, http.MethodDelete, "ghost", ""); rec.Code != http.StatusNotFound {
		t.Errorf("deleting an unknown id = %d", rec.Code)
	}
}

func TestProfileHandlers_SchemaListsTheSettings(t *testing.T) {
	h := NewProfileHandlers(builtinProfiles(t))
	rec := httptest.NewRecorder()
	h.Schema(rec, httptest.NewRequest(http.MethodGet, "/admin/api/profile-schema", nil))
	var fields []struct{ Key, Kind, Help string }
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil || len(fields) != 12 || fields[0].Key != "name" {
		t.Errorf("schema = %v %v", fields, err)
	}
}

func TestProfileHandlers_GetSaysWhenAFileReplacesABuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "research.md"), []byte("---\nname: Mine\n---\nMine."), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewProfileHandlers(profile.MustCatalog(dir, logging.NewNopLogger()))
	rec := serveProfile(h, h.Get, http.MethodGet, "research", "")
	if !strings.Contains(rec.Body.String(), `"replaces_builtin":true`) || !strings.Contains(rec.Body.String(), `"source":"user"`) {
		t.Errorf("GET = %s", rec.Body)
	}
	rec = serveProfile(h, h.Delete, http.MethodDelete, "research", "")
	if !strings.Contains(rec.Body.String(), `"restored_builtin":true`) {
		t.Errorf("DELETE = %s", rec.Body)
	}
}
