// Package agents is the Agents stack (ADR-0032, plan agents-stack-watch): the watch side of what the agents on this computer do. It keeps the
// agent security record (what the agents' runs were refused, what they asked approval for, where an automation fetched from for the first time,
// and credentials found in what agents wrote) and publishes an alert when an unattended run did something the safety rules stopped.
//
// It never stores arguments, a URL path or query, an error text, a model's words or a secret. The core hands it observations as plain data,
// does the folder walk for the secrets pass, and words and sends every alert. Lock down stays in the core: the run path calls it directly.
package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// The ids of the stack, its modules, its settings document and its one route. The settings key and the file are unchanged from before the
// stack existed.
const (
	ID               = "agents"
	ActivityModuleID = "agent_activity"
	SecretsModuleID  = "secrets_pass"
	SettingsKey      = "agents"
	RoutePath        = "/admin/api/agent-watch"
)

// ProblemDropped is the fixed phrase the activity part reports once the queue was full and an event was not recorded.
const ProblemDropped = "the queue was full and some events were not recorded"

// Stack is the Agents stack.
type Stack struct {
	mu      sync.Mutex
	mcpMu   sync.Mutex // serialises the Tools layer read-modify-write of the server list
	mcp     *mcpState
	w       *watch
	now     func() time.Time
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// New returns the stack. Its queue exists from here, so what the core pushes before Start is kept for the loop.
func New() *Stack {
	s := &Stack{now: time.Now}
	s.w = newWatch(func() time.Time { return s.now() })
	return s
}

// Manifest declares the stack as data.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: ID, Title: "Agents", Purpose: "Give the agents on this computer their tools, and watch what they do.",
		Layers: []stacksdk.Layer{
			{ID: "tools", Title: "Tools", Purpose: "Tool servers the agents can call"},
			{ID: "watch", Title: "Watch", Purpose: "What the agents did"},
		},
		Modules: []stacksdk.ModuleSpec{
			{ID: MCPModuleID, Title: "MCP servers", Purpose: "Let agents use the tools of the MCP servers you list.", Layer: "tools", Sends: stacksdk.SendsInternet, StepUp: true,
				About: stacksdk.About{
					Watches: "The MCP servers you list, and the tools each one offers.",
					Never:   "No server is contacted until you add one and switch it on, and a tool asks you first unless you set its server to Allow.",
					ShowsUp: "Under Tools on the Agents page, and as steps in a run.",
				}},
			{ID: ActivityModuleID, Title: "Agent activity", Purpose: "Refused tool calls, approvals and new destinations.", Layer: "watch", Sends: stacksdk.SendsMessage, StepUp: true,
				About: stacksdk.About{
					Watches: "Refused tool calls, approvals and new destinations of the agents on this computer.",
					Never:   "It reads a record of what the agents did and does not stop them.",
					ShowsUp: "As messages and on the Agents page.",
				}},
			{ID: SecretsModuleID, Title: "Secrets pass", Purpose: "A nightly look through workspace files for credentials.", Layer: "watch", Sends: stacksdk.SendsMessage, StepUp: true,
				About: stacksdk.About{
					Watches: "Workspace files, each night, for credentials left in them.",
					Never:   "It never shows a secret, only the file and the kind.",
					ShowsUp: "As a finding and a message.",
				}},
		},
		DynamicTools: true,
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RoutePath},
			{Method: http.MethodGet, Path: RouteMCPServers},
			{Method: http.MethodPost, Path: RouteMCPServers, Access: stacksdk.AccessStepUp},
			{Method: http.MethodPut, Path: RouteMCPServers, Access: stacksdk.AccessStepUp},
			{Method: http.MethodDelete, Path: RouteMCPServers, Access: stacksdk.AccessStepUp},
			{Method: http.MethodPost, Path: RouteMCPTest, Access: stacksdk.AccessStepUp},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Storage:  []string{File},
		Observes: []string{stacksdk.ObservedRefused, stacksdk.ObservedApproval, stacksdk.ObservedFetch},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortEvents, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortSecretScan, stacksdk.PortVault, stacksdk.PortHTTP},
	}
}

// Start builds the watch over the host's ports and runs it under ctx until ctx ends or Stop is called, then connects the Tools layer's servers.
func (s *Stack) Start(ctx context.Context, h stacksdk.Host) error {
	run, err := s.startWatch(ctx, h)
	if err != nil {
		return err
	}
	s.startMCP(run, h)
	return nil
}

func (s *Stack) startWatch(ctx context.Context, h stacksdk.Host) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil, errors.New("the agents stack is already running")
	}
	s.w.bind(h)
	s.started = true
	run, cancel := context.WithCancel(ctx)
	s.cancel, s.done = cancel, make(chan struct{})
	go func(w *watch, done chan struct{}) {
		defer close(done)
		w.run(run)
	}(s.w, s.done)
	return run, nil
}

// Stop ends the loop and waits for it.
func (s *Stack) Stop(context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.started = false
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	s.stopMCP()
	cancel()
	<-done
	return nil
}

// ConfigChanged re-reads the Tools layer settings: the secrets pass reads its switches each time it runs, and the core gates the feed itself.
func (s *Stack) ConfigChanged(ctx context.Context) error {
	s.reconcileMCP(ctx)
	return nil
}

// Observe takes one observation the core saw. It never blocks: before Start it waits in the queue, and one that does not fit is dropped
// and counted.
func (s *Stack) Observe(o stacksdk.Observation) { s.w.observe(o) }

// Status reports whether each part is on, when the secrets pass last ran, and whether the activity part ever dropped an event.
func (s *Stack) Status(ctx context.Context) ([]stacksdk.ModuleStatus, error) {
	s.mu.Lock()
	w, started := s.w, s.started
	s.mu.Unlock()
	if !started {
		return nil, nil
	}
	set := w.settings(ctx)
	pass := stacksdk.ModuleStatus{ID: SecretsModuleID, Enabled: !set.SecretsPassOff, Every: scanEvery}
	if t := w.lastRun.Load(); t != nil {
		pass.LastRun = *t
	}
	activity := stacksdk.ModuleStatus{ID: ActivityModuleID, Enabled: !set.ActivityOff}
	if w.dropped.Load() > 0 {
		activity.Problem = ProblemDropped
	}
	return []stacksdk.ModuleStatus{activity, pass, s.mcpStatus(ctx)}, nil
}

// CallTool: the stack declares no tools.
func (s *Stack) CallTool(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
}

// Findings: the stack offers nothing to the nightly report. Its entries are shown on the Agents page and sent as alerts.
func (s *Stack) Findings(context.Context) ([]stacksdk.StackFinding, error) { return nil, nil }

// Handle answers the one route: the newest entries first, and what each automation's destinations look like. A record that cannot be read is
// an error, never an empty record.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if resp, found := s.handleMCP(ctx, req); found {
		return resp, nil
	}
	if req.Method != http.MethodGet || req.Path != RoutePath {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	s.mu.Lock()
	w, started := s.w, s.started
	s.mu.Unlock()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the agents stack is not running")
	}
	rec, err := w.store.read()
	if err != nil {
		w.report(err)
		return failure(http.StatusInternalServerError, "failed to read the agent security record"), nil
	}
	return ok(buildView(rec, LearnFor, w.now())), nil
}

func ok(v any) stacksdk.Response {
	body, err := json.Marshal(v)
	if err != nil {
		return failure(http.StatusInternalServerError, "the answer could not be written")
	}
	return stacksdk.Response{Status: http.StatusOK, ContentType: "application/json", Body: append(body, '\n')}
}

func failure(status int, msg string) stacksdk.Response {
	body, _ := json.Marshal(map[string]string{"error": msg})
	return stacksdk.Response{Status: status, ContentType: "application/json", Body: append(body, '\n')}
}
