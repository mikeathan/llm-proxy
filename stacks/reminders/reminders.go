// Package reminders is the Reminders stack (ADR-0068): reminders the owner asks for, kept and delivered word for word at the right time. There is no
// model at fire time: the text the owner asked for is the text that is sent, through the core's notify port to the connectors the owner enabled.
//
// The stack keeps its own database (reminders.db in the data folder), the schedule and the delivery loop, the natural-language reading of
// "/remind tomorrow 9am call the bank", and the routes the Reminders screen reads. The core keeps what the run path needs: the assistant's tool
// definitions, the /remind command's entry and the conversation's zone, and calls the stack through its declared tools.
package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// The ids of the stack, its one module, its settings document and its one file. The settings key is the one the service has always used.
const (
	ID          = "reminders"
	ModuleID    = "reminders"
	SettingsKey = "reminders"
	DBFile      = "reminders.db"
)

// ErrOff is the refusal of a change while the module is switched off.
var ErrOff = errors.New("Reminders is switched off: switch it on under Stacks")

// Settings is the stack's settings document. SkipConnectors and Timezone are the owner's, as before; Disabled is the module's switch, stored as
// an "off" flag so an install that has always had reminders keeps them on.
type Settings struct {
	SkipConnectors []string `json:"skip_connectors,omitempty"`
	Timezone       string   `json:"timezone,omitempty"`
	Disabled       bool     `json:"disabled,omitempty"`
}

// Stack is the Reminders stack.
type Stack struct {
	mu      sync.Mutex
	h       stacksdk.Host
	store   *Store
	svc     *Service
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
	// legacy are the rows the service kept in its main database before this stack existed, staged by the core before Start (StageLegacy).
	legacy []Reminder
	// lastRun is the last time the delivery loop looked at the due reminders.
	lastRun time.Time
	clock   Clock
}

// New returns the stack, on the wall clock.
func New() *Stack { return &Stack{clock: SystemClock()} }

// NewWithClock returns the stack on a clock a test drives.
func NewWithClock(c Clock) *Stack { return &Stack{clock: c} }

// Manifest declares the stack as data.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: ID, Title: "Reminders", Purpose: "Remind you of things, word for word, at the right time.",
		Layers: []stacksdk.Layer{{ID: "remind", Title: "Remind", Purpose: "What you asked to be reminded of"}},
		Modules: []stacksdk.ModuleSpec{
			{ID: ModuleID, Title: "Reminders", Purpose: "Keep the reminders you ask for and send them on time.", Layer: "remind", Sends: stacksdk.SendsMessage,
				About: stacksdk.About{
					Watches:  "The reminders you set, here, in chat or from a connector.",
					Never:    "It sends your own words, unchanged: no model writes or rewrites a reminder when it fires.",
					ShowsUp:  "As a message on your connectors, in the web page, and on the Reminders card.",
					Settings: true,
				}},
		},
		Tools: []stacksdk.ToolSpec{
			{Name: ToolSchedule, Category: "reminders", Description: "Store one reminder for the assistant.", Schema: json.RawMessage(`{"type":"object"}`)},
			{Name: ToolList, Category: "reminders", Description: "List the open reminders.", Schema: json.RawMessage(`{"type":"object"}`)},
			{Name: ToolCancel, Category: "reminders", Description: "Cancel an open reminder.", Schema: json.RawMessage(`{"type":"object"}`)},
			{Name: ToolCommand, Category: "reminders", Description: "Read the text after /remind and store it or describe it.", Schema: json.RawMessage(`{"type":"object"}`)},
		},
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RouteList},
			{Method: http.MethodGet, Path: RouteEvents},
			{Method: http.MethodPost, Path: RouteList},
			{Method: http.MethodPut, Path: RouteItem},
			{Method: http.MethodDelete, Path: RouteItem},
			{Method: http.MethodPost, Path: RoutePause},
			{Method: http.MethodPost, Path: RouteResume},
			{Method: http.MethodPost, Path: RouteSkip},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Storage:  []string{DBFile},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortNotify, stacksdk.PortSchedule},
	}
}

// settings reads the settings document; a document that cannot be read is the default, and the owner's reminders carry on.
func (s *Stack) settings(ctx context.Context) Settings {
	var set Settings
	if s.h.Config == nil {
		return set
	}
	raw, err := s.h.Config.Load(ctx)
	if err != nil || len(raw) == 0 {
		return set
	}
	_ = json.Unmarshal(raw, &set)
	return set
}

// owner reads the settings with a short deadline, for the service's callbacks that carry no context of their own.
func (s *Stack) owner() Settings {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.settings(ctx)
}

// Start opens the database, builds the service over the host's ports and runs its delivery loop under ctx until ctx ends or Stop is called.
func (s *Stack) Start(ctx context.Context, h stacksdk.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("the reminders stack is already running")
	}
	fp, ok := h.DataDir.(stacksdk.FilePather)
	if !ok {
		return errors.New("reminders: the data folder cannot name a file")
	}
	path, err := fp.Path(DBFile)
	if err != nil {
		return err
	}
	store, err := OpenStore(path)
	if err != nil {
		return err
	}
	s.h, s.store = h, store
	// The old rows are copied here, before the loop runs and before a route or a tool can answer, so nothing can be set in this file ahead of
	// them and take an id they hold.
	if err := s.importStaged(ctx); err != nil {
		_ = store.Close()
		s.h, s.store = stacksdk.Host{}, nil
		return err
	}
	s.svc = NewService(store, notifier{n: h.Notify}, notice{log: h.Log}, s.clock, Config{
		Schedule: h.Schedule,
		Skipped:  func() []string { return s.owner().SkipConnectors },
		Enabled:  func() bool { return !s.owner().Disabled },
		Log:      h.Log,
		OnRun:    func(at time.Time) { s.mu.Lock(); s.lastRun = at; s.mu.Unlock() },
	})
	run, cancel := context.WithCancel(ctx)
	s.cancel, s.done, s.started = cancel, make(chan struct{}), true
	go func(svc *Service, done chan struct{}) {
		defer close(done)
		svc.Run(run)
	}(s.svc, s.done)
	return nil
}

// Stop ends the loop, waits for it and closes the database.
func (s *Stack) Stop(context.Context) error {
	s.mu.Lock()
	cancel, done, store := s.cancel, s.done, s.store
	s.cancel, s.done, s.store, s.svc, s.started = nil, nil, nil, nil, false
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	if store != nil {
		return store.Close()
	}
	return nil
}

// ConfigChanged wakes the loop: the switch or the skipped connectors may have changed, and a reminder due while the module was off is handled now.
func (s *Stack) ConfigChanged(context.Context) error {
	s.mu.Lock()
	svc := s.svc
	s.mu.Unlock()
	if svc != nil {
		svc.poke()
	}
	return nil
}

// running returns the service, or the refusal that the stack is not running.
func (s *Stack) running() (*Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.svc == nil {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "the reminders stack is not running")
	}
	return s.svc, nil
}

// zone resolves the zone a request named, else the owner's, else UTC; an unknown name is an error naming it.
func (s *Stack) zone(requested string) (*time.Location, error) {
	return ResolveZone(requested, s.owner().Timezone)
}

// Status reports whether the module is on, and when the loop last looked at the due reminders.
func (s *Stack) Status(ctx context.Context) ([]stacksdk.ModuleStatus, error) {
	s.mu.Lock()
	started, last := s.started, s.lastRun
	s.mu.Unlock()
	if !started {
		return nil, nil
	}
	return []stacksdk.ModuleStatus{{ID: ModuleID, Enabled: !s.settings(ctx).Disabled, LastRun: last}}, nil
}

// CallTool runs one of the declared tools.
func (s *Stack) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	svc, err := s.running()
	if err != nil {
		return nil, err
	}
	return tools{svc: svc, zone: s.zone}.call(ctx, name, args)
}

// Findings: the stack offers nothing to the nightly report.
func (s *Stack) Findings(context.Context) ([]stacksdk.StackFinding, error) { return nil, nil }

// Handle answers the routes the Reminders screen reads.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	svc, err := s.running()
	if err != nil {
		return stacksdk.Response{}, err
	}
	a := api{svc: svc, zone: s.zone, log: s.logLine}
	return a.handle(ctx, req)
}

func (s *Stack) logLine(level stacksdk.Level, msg string, fields ...string) {
	if s.h.Log != nil {
		s.h.Log.Log(level, msg, fields...)
	}
}

// StageLegacy hands the stack the reminders the service kept in its main database before this stack existed, as plain data, for Start to copy in
// once (see Store.ImportLegacy). It must be called before Start; rows staged later are not copied.
func (s *Stack) StageLegacy(rows []Reminder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.legacy = slices.Clone(rows)
}

// importStaged copies the staged rows, once, and logs how many. It is called with the stack's lock held, from Start.
func (s *Stack) importStaged(ctx context.Context) error {
	if len(s.legacy) == 0 {
		return nil
	}
	n, err := s.store.ImportLegacy(ctx, s.legacy)
	s.legacy = nil
	if err != nil {
		return fmt.Errorf("reminders: the old reminders could not be copied: %w", err)
	}
	if n > 0 {
		s.logLine(stacksdk.LevelInfo, "the reminders of the main database were copied into the stack", "count", strconv.Itoa(n))
	}
	return nil
}

// EraseAll removes every reminder, for a factory reset. It works whether or not the stack is running (a stack that is not running has no open file,
// and a file that is not there is already empty), and wakes the loop so no timer is left for a reminder that is gone.
func (s *Stack) EraseAll(ctx context.Context) error {
	s.mu.Lock()
	store, svc := s.store, s.svc
	s.mu.Unlock()
	if store == nil {
		return nil
	}
	if err := store.DeleteAll(ctx); err != nil {
		return err
	}
	if svc != nil {
		svc.poke()
	}
	return nil
}

// notifier gives the service the notify port as the Deliverer it was built for.
type notifier struct{ n stacksdk.Notify }

func (d notifier) Deliver(ctx context.Context, connector, message string) error {
	return d.n.Send(ctx, connector, message)
}

func (d notifier) Connectors() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := d.n.Channels(ctx)
	if err != nil {
		return nil
	}
	return list
}

// notice records each delivery outcome. The persisted reminder row is the in-app notice (served by the events route); the log line carries the id
// and outcome but never the message, which is the owner's own text.
type notice struct{ log stacksdk.Logger }

func (n notice) Notify(_ context.Context, r Reminder, event string) {
	if n.log != nil {
		n.log.Log(stacksdk.LevelInfo, "reminder", "id", itoa(r.ID), "event", event, "channel", r.Connector)
	}
}

// itoa is strconv.FormatInt for a log field.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
