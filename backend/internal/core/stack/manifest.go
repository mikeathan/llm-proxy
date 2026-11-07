package stack

import (
	"fmt"

	"llm-proxy-stacks/sdk"
)

// A stack package describes itself in a stacksdk.Manifest, which is data. This file reads that data into the registry's own types. The
// switches and readiness checks are not data: the core wires them in (Wiring), so a manifest cannot reach the settings it names.

// Wiring is what the core supplies for one module a manifest declares.
type Wiring struct {
	Switch Switch
	// Ready says whether the module can work now; nil means always ready.
	Ready func() (bool, string)
}

// Wire returns the wiring for a module id, or false when the core has none.
type Wire func(moduleID string) (Wiring, bool)

// StackFromManifest reads a manifest's stack: its layers and actions.
func StackFromManifest(m stacksdk.Manifest) Stack {
	s := Stack{ID: StackID(m.ID), Title: m.Title, Purpose: m.Purpose}
	for _, l := range m.Layers {
		s.Layers = append(s.Layers, Layer{ID: l.ID, Title: l.Title, Purpose: l.Purpose})
	}
	for _, a := range m.Actions {
		s.Actions = append(s.Actions, Action{ID: a.ID, Title: a.Title, Purpose: a.Purpose})
	}
	return s
}

// ModulesFromManifest reads a manifest's modules and binds each to the core's wiring. It validates the manifest first and refuses a
// module the core has no wiring for; New then checks the whole description as it always did.
func ModulesFromManifest(m stacksdk.Manifest, wire Wire) ([]Module, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	mods := make([]Module, 0, len(m.Modules))
	for _, spec := range m.Modules {
		w, ok := wire(spec.ID)
		if !ok {
			return nil, fmt.Errorf("stack %q: module %q has nothing wired to it", m.ID, spec.ID)
		}
		mods = append(mods, Module{
			ID: spec.ID, Title: spec.Title, Purpose: spec.Purpose,
			About: About{Watches: spec.About.Watches, Never: spec.About.Never, ShowsUp: spec.About.ShowsUp, Settings: spec.About.Settings},
			Finds: spec.Finds, Stack: StackID(m.ID), Layer: spec.Layer, Sends: Traffic(spec.Sends), StepUp: spec.StepUp,
			Needs: spec.Needs, Monitors: spec.Monitors, Switch: w.Switch, Ready: w.Ready,
		})
	}
	return mods, nil
}
