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
	// Finds are the ids of the findings the module may offer through Stack.Findings (ADR-0036). The core words them from its own table.
	Finds []string `json:"finds,omitempty"`
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
	// Action names the privileged-helper catalogue action this route may request (ADR-0034). It is a name, not a declaration: the core
	// checks it against its own closed catalogue, requires AccessStepUp, decodes the request body as exactly that action's parameters
	// before the stack sees it, and hands the stack a one-use grant in Request.Grant. A route cannot add an action.
	Action string `json:"action,omitempty"`
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
	// Storage lists, by plain file name, the files in the data folder that this stack owns (D10). The core refuses a name that two
	// stacks claim or that one of its own stores uses, and a stack leaves the folder with exactly these files.
	Storage []string `json:"storage,omitempty"`
	// Looks lists the ids of the modules that answer a Looker request during the nightly report: sensors that send to devices (ADR-0038).
	// The core's registry defines the module itself; this only says which ids the stack will answer for. A stack that lists one must
	// require PortScopedGuard.
	Looks []string `json:"looks,omitempty"`
	// Observes lists the kinds of observation (Observed*) the stack takes in: the service pushes each one it sees, as plain data, to a stack that
	// implements Observer. A stack that lists none is never pushed one.
	Observes []string `json:"observes,omitempty"`
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
	PortFetch     = "fetch"
	PortGateway   = "gateway"
	PortIsolation = "isolation"
	// PortPrivileged is the request channel to the privileged helper: a stack passes back the grant the core gave it for a route's action.
	PortPrivileged = "privileged"
	// PortScopedGuard is the guarded dial that first applies the operator's declared scan scope: a stack that sends to local devices
	// uses it, so it cannot reach an address the operator did not declare (ADR-0038).
	PortScopedGuard = "scopedguard"
	// PortSecretScan asks the core to look through the files the agents wrote for credentials and report only where and what kind: the stack
	// never reads a workspace folder and no file's bytes cross the port (Constitution IV.6).
	PortSecretScan = "secretscan"
)

var portNames = []string{PortConfig, PortEvents, PortLog, PortDataDir, PortGuard, PortFetch, PortGateway, PortIsolation, PortPrivileged, PortScopedGuard, PortSecretScan}

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
	for _, check := range []func() error{m.checkLayers, m.checkModules, m.checkTools, m.checkRoutes, m.checkStorage, m.checkLooks, m.checkObserves} {
		if err := check(); err != nil {
			return err
		}
	}
	for _, p := range m.Requires {
		if !slices.Contains(portNames, p) {
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
	if err := m.checkFinds(); err != nil {
		return nil, err
	}
	return byID, nil
}

// checkFinds holds each declared finding id to a clean id that one module claims once.
func (m Manifest) checkFinds() error {
	seen := map[string]string{}
	for _, mod := range m.Modules {
		for _, id := range mod.Finds {
			if !idPattern.MatchString(id) {
				return fmt.Errorf("manifest %q: module %q finds %q, which is not a clean finding id", m.ID, mod.ID, id)
			}
			if other, dup := seen[id]; dup {
				return fmt.Errorf("manifest %q: finding %q is claimed by modules %q and %q", m.ID, id, other, mod.ID)
			}
			seen[id] = mod.ID
		}
	}
	return nil
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
		if err := m.checkRouteAction(rt); err != nil {
			return err
		}
		seen[key] = true
	}
	return nil
}

// checkRouteAction holds a route that names a privileged action to the shape the grant needs: a step-up write, with no wildcard (the body
// is the action's parameters and nothing in the path), in a stack that requires the privileged port.
func (m Manifest) checkRouteAction(rt RouteSpec) error {
	if rt.Action == "" {
		return nil
	}
	key := rt.Method + " " + rt.Path
	switch {
	case rt.Level() != AccessStepUp:
		return fmt.Errorf("manifest %q: route %q names an action, so it must be %q", m.ID, key, AccessStepUp)
	case rt.Method == "GET":
		return fmt.Errorf("manifest %q: route %q names an action, so it cannot be a read", m.ID, key)
	case len(rt.Params()) > 0:
		return fmt.Errorf("manifest %q: route %q names an action, so its body is the action's parameters and its path has no wildcard", m.ID, key)
	case !slices.Contains(m.Requires, PortPrivileged):
		return fmt.Errorf("manifest %q: route %q names an action, so the stack must require the %q port", m.ID, key, PortPrivileged)
	}
	return nil
}

// checkLooks holds each sensor id to a clean id declared once, and holds a stack with a sensor to the scoped dial.
func (m Manifest) checkLooks() error {
	seen := map[string]bool{}
	for _, id := range m.Looks {
		if !idPattern.MatchString(id) {
			return fmt.Errorf("manifest %q: look %q must be lower-case letters, digits and underscores", m.ID, id)
		}
		if seen[id] {
			return fmt.Errorf("manifest %q: look %q is listed twice", m.ID, id)
		}
		seen[id] = true
	}
	if len(m.Looks) > 0 && !slices.Contains(m.Requires, PortScopedGuard) {
		return fmt.Errorf("manifest %q: a stack with a sensor must require the %q port", m.ID, PortScopedGuard)
	}
	return nil
}

// checkObserves holds each observed kind to a clean id declared once.
func (m Manifest) checkObserves() error {
	seen := map[string]bool{}
	for _, id := range m.Observes {
		if !idPattern.MatchString(id) {
			return fmt.Errorf("manifest %q: observation %q must be lower-case letters, digits and underscores", m.ID, id)
		}
		if seen[id] {
			return fmt.Errorf("manifest %q: observation %q is listed twice", m.ID, id)
		}
		seen[id] = true
	}
	return nil
}

// checkStorage holds each claimed file to a plain name in the data folder, claimed once.
func (m Manifest) checkStorage() error {
	seen := map[string]bool{}
	for _, name := range m.Storage {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			return fmt.Errorf("manifest %q: storage %q must be a plain file name in the data folder", m.ID, name)
		}
		if seen[name] {
			return fmt.Errorf("manifest %q: storage %q is listed twice", m.ID, name)
		}
		seen[name] = true
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
