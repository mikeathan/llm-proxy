package stacksdk

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Traffic is what a module sends or opens, in the words the screen uses.
type Traffic string

// The kinds of traffic a module declares.
const (
	SendsNothing  Traffic = "nothing"
	SendsLAN      Traffic = "lan"
	SendsInternet Traffic = "internet"
	SendsMessage  Traffic = "message" // a message to the operator's connectors
	Listens       Traffic = "listens" // opens a listening socket on this computer
)

// Valid reports whether the value is one of the kinds above.
func (t Traffic) Valid() bool {
	return slices.Contains([]Traffic{SendsNothing, SendsLAN, SendsInternet, SendsMessage, Listens}, t)
}

// Layer is one tier of a stack, bottom first.
type Layer struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

// Action is something a stack offers that is not a switch.
type Action struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

// About says what a module watches, what it never does and where its results appear.
type About struct {
	Watches  string `json:"watches"`
	Never    string `json:"never"`
	ShowsUp  string `json:"shows_up"`
	Settings bool   `json:"settings,omitempty"`
}

// ModuleSpec describes one switchable capability as data. The switch itself is wired by the core.
type ModuleSpec struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Purpose  string   `json:"purpose"`
	About    About    `json:"about"`
	Layer    string   `json:"layer"`
	Sends    Traffic  `json:"sends"`
	StepUp   bool     `json:"step_up,omitempty"`
	Needs    []string `json:"needs,omitempty"`
	Monitors []string `json:"monitors,omitempty"`
	Finds    []string `json:"finds,omitempty"`
}

// ToolSpec declares an agent tool as data: the handler is called by name with JSON arguments and returns JSON.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Schema      json.RawMessage `json:"schema"` // JSON Schema of the arguments
}

// Access is who may call a route. A route is never public: the core signs every caller in first, and a route cannot lower that. There
// is no value for "open" (ADR-0033).
type Access string

// The access levels a route may declare.
const (
	// AccessAdmin: a signed-in operator or token. An empty Access means this.
	AccessAdmin Access = "admin"
	// AccessStepUp: a write that needs the passphrase again, shortly before (a destructive or outward-facing action).
	AccessStepUp Access = "step_up"
)

// RouteSpec declares one HTTP route of a stack as data. The core mounts it under the same method and path, authenticates the caller,
// checks the access level, and hands the stack a Request; the stack never sees the connection (ADR-0032, plan P3b D5).
type RouteSpec struct {
	// Method is GET, POST, PUT, PATCH or DELETE.
	Method string `json:"method"`
	// Path is the full path, starting with "/", with {name} wildcards for the parts the stack reads from Request.Params.
	Path string `json:"path"`
	// Access defaults to AccessAdmin. A write that needs the passphrase again declares AccessStepUp.
	Access Access `json:"access,omitempty"`
}

// Level is the access the route needs: what it declares, or admin when it declares nothing.
func (r RouteSpec) Level() Access {
	if r.Access == "" {
		return AccessAdmin
	}
	return r.Access
}

// Params lists the {name} wildcards of the path, in order.
func (r RouteSpec) Params() []string {
	var names []string
	rest := r.Path
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			return names
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return names
		}
		names = append(names, rest[i+1:i+j])
		rest = rest[i+j+1:]
	}
}

// SettingsSchema names the stack's settings document. The persisted key is the stack's own and does not change when a stack moves.
type SettingsSchema struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
}

// Manifest is everything the core needs to know about a stack before it runs it. It declares no privilege: there is no field for
// one (ADR-0033).
type Manifest struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Purpose  string         `json:"purpose"`
	Layers   []Layer        `json:"layers"`
	Actions  []Action       `json:"actions,omitempty"`
	Modules  []ModuleSpec   `json:"modules"`
	Tools    []ToolSpec     `json:"tools,omitempty"`
	Routes   []RouteSpec    `json:"routes,omitempty"`
	Settings SettingsSchema `json:"settings"`
	// Requires lists the ports the stack uses; each must be one of the Port* names below.
	Requires []string `json:"requires"`
}

// Names of the ports a manifest may require.
const (
	PortConfig    = "config"
	PortEvents    = "events"
	PortLog       = "log"
	PortDataDir   = "datadir"
	PortGuard     = "guard"
	PortIsolation = "isolation"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Validate refuses an inconsistent manifest: a bad or duplicate id, a module on a layer that does not exist, a need that is not a
// module of this stack, an unknown kind of traffic or port, a tool without a schema.
func (m Manifest) Validate() error {
	if !idPattern.MatchString(m.ID) || m.Title == "" {
		return fmt.Errorf("manifest %q: the id must be lower-case letters, digits and underscores, and the title is required", m.ID)
	}
	if m.Settings.Key == "" {
		return fmt.Errorf("manifest %q: settings key is required", m.ID)
	}
	if err := m.checkLayers(); err != nil {
		return err
	}
	if err := m.checkModules(); err != nil {
		return err
	}
	if err := m.checkTools(); err != nil {
		return err
	}
	if err := m.checkRoutes(); err != nil {
		return err
	}
	for _, p := range m.Requires {
		if !slices.Contains([]string{PortConfig, PortEvents, PortLog, PortDataDir, PortGuard, PortIsolation}, p) {
			return fmt.Errorf("manifest %q requires %q, which is not a port", m.ID, p)
		}
	}
	return nil
}

func (m Manifest) checkLayers() error {
	seen := map[string]bool{}
	for _, l := range m.Layers {
		if l.ID == "" || seen[l.ID] {
			return fmt.Errorf("manifest %q: a layer has no id or two share one (%q)", m.ID, l.ID)
		}
		seen[l.ID] = true
	}
	return nil
}

func (m Manifest) checkModules() error {
	layerAt := map[string]int{}
	for i, l := range m.Layers {
		layerAt[l.ID] = i
	}
	byID, err := m.indexModules(layerAt)
	if err != nil {
		return err
	}
	for _, mod := range m.Modules {
		for _, id := range mod.Needs {
			need, ok := byID[id]
			if !ok || id == mod.ID || layerAt[need.Layer] > layerAt[mod.Layer] {
				return fmt.Errorf("manifest %q: module %q needs %q, which is not a module of this stack on its layer or below", m.ID, mod.ID, id)
			}
		}
	}
	return nil
}

// indexModules checks each module on its own (complete, unique, on a layer that exists) and returns them by id.
func (m Manifest) indexModules(layerAt map[string]int) (map[string]ModuleSpec, error) {
	byID := map[string]ModuleSpec{}
	for _, mod := range m.Modules {
		if mod.ID == "" || mod.Title == "" || !mod.Sends.Valid() {
			return nil, fmt.Errorf("manifest %q: module %q needs an id, a title and a kind of traffic", m.ID, mod.ID)
		}
		if _, dup := byID[mod.ID]; dup {
			return nil, fmt.Errorf("manifest %q: module %q is listed twice", m.ID, mod.ID)
		}
		if _, ok := layerAt[mod.Layer]; !ok {
			return nil, fmt.Errorf("manifest %q: module %q is on layer %q, which does not exist", m.ID, mod.ID, mod.Layer)
		}
		byID[mod.ID] = mod
	}
	return byID, nil
}

func (m Manifest) checkRoutes() error {
	seen := map[string]bool{}
	for _, rt := range m.Routes {
		key := rt.Method + " " + rt.Path
		switch {
		case !slices.Contains(routeMethods, rt.Method):
			return fmt.Errorf("manifest %q: route %q has a method that is not one of %v", m.ID, key, routeMethods)
		case !strings.HasPrefix(rt.Path, "/") || strings.Contains(rt.Path, "//") || strings.Contains(rt.Path, "..") || strings.ContainsAny(rt.Path, "?# "):
			return fmt.Errorf("manifest %q: route %q needs a clean path starting with /", m.ID, key)
		case rt.Access != "" && rt.Access != AccessAdmin && rt.Access != AccessStepUp:
			return fmt.Errorf("manifest %q: route %q has access %q; use %q or %q", m.ID, key, rt.Access, AccessAdmin, AccessStepUp)
		case seen[key]:
			return fmt.Errorf("manifest %q: route %q is listed twice", m.ID, key)
		}
		seen[key] = true
	}
	return nil
}

var routeMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

func (m Manifest) checkTools() error {
	seen := map[string]bool{}
	for _, t := range m.Tools {
		if t.Name == "" || seen[t.Name] || t.Category == "" || !json.Valid(t.Schema) {
			return fmt.Errorf("manifest %q: tool %q needs a unique name, a category and a valid JSON schema", m.ID, t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}
