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
	"time"

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
	// OwnNetworks are the names of the operator's own networks (see NetworksPath).
	OwnNetworks []string `json:"own_networks,omitempty"`
	// Channels, WindowSeconds and CycleSeconds shape a listening run (Plan applies the defaults and caps): the channels to visit, how long one
	// window lasts and the time from one cycle's start to the next.
	Channels      []int `json:"channels,omitempty"`
	WindowSeconds int   `json:"window_seconds,omitempty"`
	CycleSeconds  int   `json:"cycle_seconds,omitempty"`
}

// View is what the Sensors page shows.
type View struct {
	// Enabled is the stack's switch.
	Enabled bool     `json:"enabled"`
	Sensors []Sensor `json:"sensors"`
	// Reason is a fixed phrase, set when there is nothing to list.
	Reason string `json:"reason,omitempty"`
	// Unused counts what could be built next: capabilities the radio has and the stack does not use yet, and radios nothing drives.
	Unused int `json:"unused"`
}

// Stack is the Radio stack.
type Stack struct {
	sys     fs.FS
	mu      sync.Mutex
	h       stacksdk.Host
	started bool
	store   *Store
	// run is the listening run, nil until one is started (listen.go).
	run *run
	// rec is the recorder over store (storage.go).
	rec *Recorder
	// clock, sleep and maxFrames are what a test replaces.
	clock     func() time.Time
	sleep     func(context.Context, time.Duration) error
	maxFrames int
}

func (s *Stack) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

func (s *Stack) wait(ctx context.Context, d time.Duration) error {
	if s.sleep != nil {
		return s.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Stack) frameCap() int {
	if s.maxFrames > 0 {
		return s.maxFrames
	}
	return defaultFrameCap
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
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RoutePath},
			{Method: http.MethodGet, Path: ListenPath}, {Method: http.MethodPost, Path: ListenPath}, {Method: http.MethodDelete, Path: ListenPath},
			{Method: http.MethodGet, Path: PlanPath},
			{Method: http.MethodGet, Path: DevicesPath}, {Method: http.MethodPost, Path: DevicesPath}, {Method: http.MethodDelete, Path: DevicePath},
			{Method: http.MethodPost, Path: ForgetPath},
			{Method: http.MethodGet, Path: AirPath}, {Method: http.MethodGet, Path: AirDevicesPath}, {Method: http.MethodGet, Path: AirwavesPath},
			{Method: http.MethodGet, Path: PresencePath}, {Method: http.MethodGet, Path: DigestPath},
			{Method: http.MethodGet, Path: NetworksPath}, {Method: http.MethodPut, Path: NetworksPath},
			{Method: http.MethodPost, Path: NetworkConfirmPath}, {Method: http.MethodDelete, Path: NetworkPath},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortVault, stacksdk.PortPrivileged},
		Storage:  []string{DBFile},
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

// Stop ends a listening run, if one is going, and forgets the host.
func (s *Stack) Stop(context.Context) error {
	s.endRun(EndedStack)
	s.mu.Lock()
	s.run = nil
	defer s.mu.Unlock()
	s.h, s.started = stacksdk.Host{}, false
	return s.closeStoreLocked()
}

// ConfigChanged ends a listening run when the stack has been switched off; the settings are read each time they are needed.
func (s *Stack) ConfigChanged(ctx context.Context) error {
	if h, started := s.snapshot(); started && !s.settings(ctx, h).Enabled {
		s.endRun(EndedSwitchedOff)
	}
	return nil
}

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
	switch req.Path {
	case ListenPath, PlanPath:
		return s.handleListen(ctx, req)
	case DigestPath:
		return s.handleDigest(ctx, req)
	case DevicesPath, DevicePath, ForgetPath, PresencePath:
		return s.handleDevices(ctx, req)
	case AirPath, AirDevicesPath, AirwavesPath, NetworksPath, NetworkPath, NetworkConfirmPath:
		return s.handleAir(ctx, req)
	}
	if req.Method != http.MethodGet || req.Path != RoutePath {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	snap := Discover(s.sys)
	snap.Sensors = Describe(snap.Sensors, s.lastProbe(ctx), s.listeningAdapter(), s.phyOf)
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
			if c.Use == UseNotBuilt || c.Use == UseNoDriver {
				n++
			}
		}
	}
	return n
}

// lastProbe is the last read of the adapters' capabilities, or nil when none was made (or there is no database yet: reading the page creates none).
func (s *Stack) lastProbe(ctx context.Context) *ProbeRead {
	st, err := s.existingStore()
	if err != nil || st == nil {
		return nil
	}
	var read ProbeRead
	if ok, err := st.GetDoc(ctx, probeDoc, &read); err != nil || !ok {
		return nil
	}
	return &read
}

// listeningAdapter is the adapter a run is listening with now, or "".
func (s *Stack) listeningAdapter() string {
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	if r == nil {
		return ""
	}
	if st := r.snapshot(); st.Listening {
		return st.Adapter
	}
	return ""
}
