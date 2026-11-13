// Package radio is the Radio stack (ADR-0052, plan radio-stack): passive wireless sensing with any number of adapters. This is the R1
// skeleton: it finds the radios the system's own records show, describes what each could do, and says which of those it uses. It sends nothing,
// captures nothing, stores nothing and starts no goroutine: with no adapter, or with the stack off, it is idle by construction.
//
// Capture, the store, the tracker and the findings arrive in later phases (docs/PLANS/cross-cutting/radio-stack.md §9).
package radio

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"sync"

	"llm-proxy-stacks/sdk"
)

// The ids of the stack, its module, its settings document and its one route.
const (
	ID              = "radio"
	SensorsModuleID = "radio_sensors"
	SettingsKey     = "radio"
	RoutePath       = "/admin/api/radio/sensors"
)

// Settings is the stack's settings document. The stack is off until the operator switches it on: it is a new kind of sensor, and a
// computer that never opted in listens to nothing.
type Settings struct {
	Enabled bool `json:"enabled"`
}

// View is what the Sensors page shows.
type View struct {
	// Enabled is the stack's switch.
	Enabled bool     `json:"enabled"`
	Sensors []Sensor `json:"sensors"`
	// Reason is a fixed phrase, set when there is nothing to list.
	Reason string `json:"reason,omitempty"`
	// Unused counts the capabilities found and not used yet (by design, not built, or no driver): the operator's list for later.
	Unused int `json:"unused"`
}

// Stack is the Radio stack.
type Stack struct {
	sys     fs.FS
	mu      sync.Mutex
	h       stacksdk.Host
	started bool
}

// New returns the stack, reading the system's records from sys. Pass nil to read the real /sys.
func New(sys fs.FS) *Stack {
	if sys == nil {
		sys = os.DirFS("/sys")
	}
	return &Stack{sys: sys}
}

// Manifest declares the stack as data. It declares one layer and one module: the others are added by the phase that builds them.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: ID, Title: "Radio", Purpose: "Listen to the radio around this computer, with the adapters you plug in.",
		Layers: []stacksdk.Layer{{ID: "sensors", Title: "Sensors", Purpose: "The radios this computer has, and what each can do"}},
		Modules: []stacksdk.ModuleSpec{
			{ID: SensorsModuleID, Title: "Radio sensors", Purpose: "Find the radios on this computer and say what each can do.", Layer: "sensors", Sends: stacksdk.SendsNothing,
				About: stacksdk.About{
					Watches: "The wireless adapters plugged into this computer, from the system's own records.",
					Never:   "It sends nothing and listens to nothing yet. With no adapter it does nothing at all.",
					ShowsUp: "On the Radio page.",
				}},
		},
		Routes:   []stacksdk.RouteSpec{{Method: http.MethodGet, Path: RoutePath}},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog},
	}
}

// Start remembers the host. It starts no goroutine: discovery runs when the page asks.
func (s *Stack) Start(_ context.Context, h stacksdk.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("the radio stack is already running")
	}
	s.h, s.started = h, true
	return nil
}

// Stop forgets the host.
func (s *Stack) Stop(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h, s.started = stacksdk.Host{}, false
	return nil
}

// ConfigChanged has nothing to re-read: the switch is read each time it is asked.
func (s *Stack) ConfigChanged(context.Context) error { return nil }

// Status reports the module's switch; nothing runs in the background, so there is no last run.
func (s *Stack) Status(ctx context.Context) ([]stacksdk.ModuleStatus, error) {
	h, started := s.snapshot()
	if !started {
		return nil, nil
	}
	return []stacksdk.ModuleStatus{{ID: SensorsModuleID, Enabled: s.settings(ctx, h).Enabled}}, nil
}

// CallTool: the stack declares no tools yet.
func (s *Stack) CallTool(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
}

// Findings: the stack offers nothing to the nightly report yet.
func (s *Stack) Findings(context.Context) ([]stacksdk.StackFinding, error) { return nil, nil }

// Handle answers the one route: discovery, read fresh each time (a few small files), so a plugged-in adapter shows on the next look.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if req.Method != http.MethodGet || req.Path != RoutePath {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	snap := Discover(s.sys)
	v := View{Enabled: s.settings(ctx, h).Enabled, Sensors: snap.Sensors, Reason: snap.Reason, Unused: unusedCount(snap.Sensors)}
	body, err := json.Marshal(v)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInternal, "the answer could not be written")
	}
	return stacksdk.Response{Status: http.StatusOK, ContentType: "application/json", Body: append(body, '\n')}, nil
}

func (s *Stack) snapshot() (stacksdk.Host, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h, s.started
}

// settings reads the document; a missing or unreadable one is the default (off).
func (s *Stack) settings(ctx context.Context, h stacksdk.Host) Settings {
	var set Settings
	if h.Config == nil {
		return set
	}
	raw, err := h.Config.Load(ctx)
	if err != nil || json.Unmarshal(raw, &set) != nil {
		return Settings{}
	}
	return set
}

func unusedCount(sensors []Sensor) int {
	n := 0
	for _, s := range sensors {
		for _, c := range s.Capabilities {
			if c.Use != UseUsed {
				n++
			}
		}
	}
	return n
}
