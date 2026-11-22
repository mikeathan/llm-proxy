package newsroom

import (
	"encoding/json"
	"net/http"

	"llm-proxy-stacks/newsroom/brief"
	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/sdk"
)

// sourceEntry is one source the topic form offers: the built-in list (no id, builtin true) and the owner's own (an id, so it can be removed).
type sourceEntry struct {
	ID       int64        `json:"id,omitempty"`
	Label    string       `json:"label"`
	Category string       `json:"category"`
	Kind     sources.Kind `json:"kind"`
	Target   string       `json:"target"`
	Builtin  bool         `json:"builtin"`
}

// sourceList is the built-in list first, then the owner's own.
func sourceList(store *brief.Store) []sourceEntry {
	out := []sourceEntry{}
	for _, e := range sources.Catalog() {
		out = append(out, sourceEntry{Label: e.Label, Category: e.Category, Kind: e.Kind, Target: e.Target, Builtin: true})
	}
	for _, m := range store.MySources() {
		out = append(out, sourceEntry{ID: m.ID, Label: m.Label, Category: brief.MySourcesCategory, Kind: m.Kind, Target: m.Target})
	}
	return out
}

// handleSourceList answers the routes that list the sources the topic form offers, add one to the owner's list and remove one from it. The built-in
// entries cannot be changed.
func handleSourceList(key string, id int64, req stacksdk.Request, store *brief.Store) (stacksdk.Response, error) {
	switch key {
	case "GET " + RouteBriefSources:
		return jsonResponse(http.StatusOK, sourceList(store))
	case "POST " + RouteBriefSources:
		var in struct {
			Label  string       `json:"label"`
			Kind   sources.Kind `json:"kind"`
			Target string       `json:"target"`
		}
		if err := json.Unmarshal(req.Body, &in); err != nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefBodyNotValid)
		}
		m, err := store.AddMySource(in.Label, in.Kind, in.Target)
		if err != nil {
			return stacksdk.Response{}, failBrief(err)
		}
		return jsonResponse(http.StatusCreated, sourceEntry{ID: m.ID, Label: m.Label, Category: brief.MySourcesCategory, Kind: m.Kind, Target: m.Target})
	case "DELETE " + RouteBriefSource:
		return noContent(store.DeleteMySource(id))
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}
