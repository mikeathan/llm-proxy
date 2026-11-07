// Package stack describes the platform's switchable capabilities as modules in layers of stacks (ADR-0028). It is a
// descriptive registry: a module's setting stays where it always lived and is reached through a Switch the application hands in,
// so the registry can never become a second source of truth. The package is a leaf; it imports nothing of the features.
package stack

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// StackID names a stack: a top-level area of the platform.
type StackID string

const (
	Network StackID = "network"
	Agents  StackID = "agents"
)

// Valid reports whether the id is well formed: lower-case letters, digits and underscores, starting with a letter. Which stacks exist
// is decided by what is registered (the manifests and definitions handed to New), so an id that names no registered stack is refused
// there, when a module is placed on it.
func (s StackID) Valid() bool { return stackIDPattern.MatchString(string(s)) }

var stackIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Traffic is what a module sends or opens, in the words the screen uses.
type Traffic string

const (
	SendsNothing  Traffic = "nothing"
	SendsLAN      Traffic = "lan"
	SendsInternet Traffic = "internet"
	// SendsMessage means the module sends a message to the operator's connectors (a phone, a chat).
	SendsMessage Traffic = "message"
	// Listens means the module opens a listening socket on this computer.
	Listens Traffic = "listens"
)

// Valid reports whether the value is one of the kinds above.
func (t Traffic) Valid() bool {
	switch t {
	case SendsNothing, SendsLAN, SendsInternet, SendsMessage, Listens:
		return true
	}
	return false
}

// Health is how a module that is switched on is doing. A module with no monitor of its own has none.
type Health string

const (
	HealthWorking Health = "working"
	HealthStopped Health = "stopped"
)

// Layer is one tier of a stack. A stack lists its layers bottom first: a module may need only modules on its own layer or below.
type Layer struct {
	ID, Title, Purpose string
}

// Action is something a stack offers that is not a switch (lock down). It is shown on the stack and never counted as a module.
type Action struct {
	ID, Title, Purpose string
}

// Stack is one top-level area: its layers, bottom first, and the actions it offers.
type Stack struct {
	ID      StackID
	Title   string
	Purpose string
	Layers  []Layer
	Actions []Action
}

// Switch is the adapter over the setting a module already has. Set does whatever the feature's own route did when it was
// switched (open or close its socket, wake its loop).
type Switch interface {
	Enabled() bool
	Set(enabled bool) error
}

// About is what a person needs to know about a module before switching it on, in three short plain sentences, and whether its details
// have a card on the stack's Settings tab. It is written once, here, and the Modules tab and the Help read it, so they cannot drift.
type About struct {
	// Watches is what the module looks at.
	Watches string `json:"watches"`
	// Never is what it does not do, or what it keeps off the wire or out of its records.
	Never string `json:"never"`
	// ShowsUp is where its results appear.
	ShowsUp string `json:"shows_up"`
	// Settings is true when the stack's Settings tab has a card for the module's details.
	Settings bool `json:"settings,omitempty"`
}

// Module is one switchable capability.
type Module struct {
	ID, Title, Purpose string
	// About says what the module watches, what it never does and where its results appear.
	About About
	// Finds are the check ids of the findings only this module produces, so a finding can say what found it.
	Finds []string
	Stack StackID
	Layer string
	Sends Traffic
	// StepUp is true when switching the module needs the passphrase confirmed shortly before.
	StepUp bool
	// Needs are the ids of modules of the same stack, on this layer or below, that must be on before this one is.
	Needs []string
	// Monitors are the ids of the monitored parts that say how the module is doing; empty when it has none.
	Monitors []string
	Switch   Switch
	// Ready says whether the module can work now and, when it cannot, why in plain words. Nil means always ready.
	Ready func() (bool, string)
}

// Errors the registry returns; callers match them with errors.Is.
var (
	ErrNotFound = errors.New("stack: no such stack or module")
	ErrNeedOff  = errors.New("stack: a module this one needs is off")
)

// NeedError is the refusal to switch a module on while one it needs is off; Need is that module's title.
type NeedError struct{ Need string }

func (e *NeedError) Error() string { return "needs " + e.Need }

func (e *NeedError) Is(target error) bool { return target == ErrNeedOff }

// Registry holds the stacks and their modules. It is built once and not changed after.
type Registry struct {
	stacks  []Stack
	modules []Module
}

// New checks the description and returns the registry. It refuses anything inconsistent, so a mistake in the wiring fails at
// start and not on a screen.
func New(stacks []Stack, modules []Module) (*Registry, error) {
	if err := checkStacks(stacks); err != nil {
		return nil, err
	}
	r := &Registry{stacks: slices.Clone(stacks), modules: slices.Clone(modules)}
	seen := map[string]bool{}
	for _, m := range r.modules {
		if err := r.checkModule(m, seen); err != nil {
			return nil, err
		}
		seen[m.ID] = true
	}
	for _, m := range r.modules {
		if err := r.checkNeeds(m); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func checkStacks(stacks []Stack) error {
	seen := map[StackID]bool{}
	for _, s := range stacks {
		if !s.ID.Valid() || seen[s.ID] {
			return fmt.Errorf("stack %q is unknown or listed twice", s.ID)
		}
		seen[s.ID] = true
		layers := map[string]bool{}
		for _, l := range s.Layers {
			if l.ID == "" || layers[l.ID] {
				return fmt.Errorf("stack %q has a layer with no id or two with the same id (%q)", s.ID, l.ID)
			}
			layers[l.ID] = true
		}
	}
	return nil
}

func (r *Registry) checkModule(m Module, seen map[string]bool) error {
	switch {
	case m.ID == "" || m.Title == "":
		return errors.New("a module has no id or no title")
	case seen[m.ID]:
		return fmt.Errorf("module %q is listed twice", m.ID)
	case m.Switch == nil:
		return fmt.Errorf("module %q has no switch", m.ID)
	case !m.Sends.Valid():
		return fmt.Errorf("module %q sends %q, which is not a kind of traffic", m.ID, m.Sends)
	}
	if _, ok := r.layerIndex(m.Stack, m.Layer); !ok {
		return fmt.Errorf("module %q is on layer %q of stack %q, which does not exist", m.ID, m.Layer, m.Stack)
	}
	return nil
}

// checkNeeds runs after every module is known: a need must exist in the same stack and sit on the module's layer or below.
func (r *Registry) checkNeeds(m Module) error {
	own, _ := r.layerIndex(m.Stack, m.Layer)
	for _, id := range m.Needs {
		need, ok := r.find(id)
		if !ok || need.Stack != m.Stack || need.ID == m.ID {
			return fmt.Errorf("module %q needs %q, which is not another module of its stack", m.ID, id)
		}
		if idx, _ := r.layerIndex(need.Stack, need.Layer); idx > own {
			return fmt.Errorf("module %q needs %q, which sits on a higher layer", m.ID, id)
		}
	}
	return nil
}

func (r *Registry) stack(id StackID) (Stack, bool) {
	for _, s := range r.stacks {
		if s.ID == id {
			return s, true
		}
	}
	return Stack{}, false
}

func (r *Registry) layerIndex(id StackID, layer string) (int, bool) {
	s, ok := r.stack(id)
	if !ok {
		return 0, false
	}
	idx := slices.IndexFunc(s.Layers, func(l Layer) bool { return l.ID == layer })
	return idx, idx >= 0
}

func (r *Registry) find(id string) (Module, bool) {
	for _, m := range r.modules {
		if m.ID == id {
			return m, true
		}
	}
	return Module{}, false
}

// Module returns the module of that stack with that id.
func (r *Registry) Module(stack StackID, id string) (Module, bool) {
	m, ok := r.find(id)
	return m, ok && m.Stack == stack
}

// Counts is how many of a stack's modules are on, stopped or blocked.
type Counts struct {
	On      int `json:"on"`
	Total   int `json:"total"`
	Stopped int `json:"stopped"`
	Blocked int `json:"blocked"`
}

// StackSummary is a stack in the list of stacks.
type StackSummary struct {
	ID      StackID `json:"id"`
	Title   string  `json:"title"`
	Purpose string  `json:"purpose"`
	Counts
}

// ActionView is an action of a stack.
type ActionView struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

// ModuleView is a module as the screen shows it.
type ModuleView struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Purpose string  `json:"purpose"`
	About   About   `json:"about"`
	Sends   Traffic `json:"sends"`
	StepUp  bool    `json:"step_up"`
	Enabled bool    `json:"enabled"`
	// Finds are the check ids of the findings only this module produces.
	Finds []string `json:"finds"`
	// Health is set only while the module is on and has a monitor.
	Health Health `json:"health,omitempty"`
	// Blocked says in plain words why the module cannot work now ("needs Presence check", or its readiness reason).
	Blocked string   `json:"blocked,omitempty"`
	Needs   []string `json:"needs"`
}

// LayerView is a tier of a stack with the modules on it.
type LayerView struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Purpose string       `json:"purpose,omitempty"`
	Modules []ModuleView `json:"modules"`
}

// StackView is a stack with its layers drawn top first, the way the screen shows them. A layer with no module is left out.
type StackView struct {
	ID      StackID      `json:"id"`
	Title   string       `json:"title"`
	Purpose string       `json:"purpose"`
	Summary Counts       `json:"summary"`
	Layers  []LayerView  `json:"layers"`
	Actions []ActionView `json:"actions"`
}

// Stacks lists every stack with its counts. stalled holds the monitored parts that have stopped.
func (r *Registry) Stacks(stalled map[string]bool) []StackSummary {
	out := make([]StackSummary, 0, len(r.stacks))
	for _, s := range r.stacks {
		out = append(out, StackSummary{ID: s.ID, Title: s.Title, Purpose: s.Purpose, Counts: r.counts(s.ID, stalled)})
	}
	return out
}

// View describes one stack: its layers top first, each module with its state, health and what blocks it.
func (r *Registry) View(id StackID, stalled map[string]bool) (StackView, error) {
	s, ok := r.stack(id)
	if !ok {
		return StackView{}, ErrNotFound
	}
	v := StackView{ID: s.ID, Title: s.Title, Purpose: s.Purpose, Layers: []LayerView{}, Actions: []ActionView{}}
	for _, l := range slices.Backward(s.Layers) {
		lv := LayerView{ID: l.ID, Title: l.Title, Purpose: l.Purpose, Modules: []ModuleView{}}
		for _, m := range r.modules {
			if m.Stack == id && m.Layer == l.ID {
				mv := r.moduleView(m, stalled) // once per module: a readiness check may touch the disk
				lv.Modules = append(lv.Modules, mv)
				v.Summary.add(mv)
			}
		}
		if len(lv.Modules) > 0 {
			v.Layers = append(v.Layers, lv)
		}
	}
	for _, a := range s.Actions {
		v.Actions = append(v.Actions, ActionView{ID: a.ID, Title: a.Title, Purpose: a.Purpose})
	}
	return v, nil
}

func (r *Registry) counts(id StackID, stalled map[string]bool) Counts {
	var c Counts
	for _, m := range r.modules {
		if m.Stack == id {
			c.add(r.moduleView(m, stalled))
		}
	}
	return c
}

// add counts one module into the totals.
func (c *Counts) add(mv ModuleView) {
	c.Total++
	if mv.Enabled {
		c.On++
	}
	if mv.Health == HealthStopped {
		c.Stopped++
	}
	if mv.Blocked != "" {
		c.Blocked++
	}
}

func (r *Registry) moduleView(m Module, stalled map[string]bool) ModuleView {
	on := m.Switch.Enabled()
	mv := ModuleView{ID: m.ID, Title: m.Title, Purpose: m.Purpose, About: m.About, Sends: m.Sends, StepUp: m.StepUp, Enabled: on, Blocked: r.blockedBy(m), Needs: slices.Clone(m.Needs), Finds: slices.Clone(m.Finds)}
	if mv.Needs == nil {
		mv.Needs = []string{}
	}
	if mv.Finds == nil {
		mv.Finds = []string{}
	}
	if on && len(m.Monitors) > 0 {
		mv.Health = HealthWorking
		if slices.ContainsFunc(m.Monitors, func(id string) bool { return stalled[id] }) {
			mv.Health = HealthStopped
		}
	}
	return mv
}

// blockedBy says what stops the module working: first the modules it needs that are off, then its own readiness.
func (r *Registry) blockedBy(m Module) string {
	if off := r.needsOff(m); len(off) > 0 {
		return "needs " + strings.Join(off, " and ")
	}
	if m.Ready != nil {
		if ok, why := m.Ready(); !ok {
			return why
		}
	}
	return ""
}

// needsOff returns the titles of the modules m needs that are switched off.
func (r *Registry) needsOff(m Module) []string {
	var off []string
	for _, id := range m.Needs {
		if need, ok := r.find(id); ok && !need.Switch.Enabled() {
			off = append(off, need.Title)
		}
	}
	return off
}

// Outcome is what a switch change left behind: on switching a module off, the titles of the modules that are on and need it.
type Outcome struct {
	Blocked []string `json:"blocked"`
}

// Switch turns a module on or off through its Switch. Switching on is refused while a module it needs is off (a NeedError names
// it); switching off is always allowed and the outcome names the modules left without something they need. A module's
// readiness never refuses a switch: it shows as blocked, as it always did.
func (r *Registry) Switch(stack StackID, id string, enabled bool) (Outcome, error) {
	m, ok := r.Module(stack, id)
	if !ok {
		return Outcome{}, ErrNotFound
	}
	if enabled {
		if off := r.needsOff(m); len(off) > 0 {
			return Outcome{}, &NeedError{Need: strings.Join(off, " and ")}
		}
	}
	if err := m.Switch.Set(enabled); err != nil {
		return Outcome{}, fmt.Errorf("switching %s: %w", m.ID, err)
	}
	out := Outcome{Blocked: []string{}}
	if !enabled {
		for _, d := range r.modules {
			if d.Stack == stack && slices.Contains(d.Needs, id) && d.Switch.Enabled() {
				out.Blocked = append(out.Blocked, d.Title)
			}
		}
	}
	return out, nil
}
