// Package network is the Network stack (ADR-0032, plan P3b). It is moving here one vertical at a time; today it holds the router DNS check.
// It imports only the SDK: everything it needs from the service comes through a port in the Host it is started with.
package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"llm-proxy-stacks/network/dnscheck"
	"llm-proxy-stacks/sdk"
)

// ID is the stack's id; the settings key and the files are unchanged from before the move.
const (
	ID       = "network"
	ModuleID = "dns_check"
	// SettingsKey is the core's `netaudit` settings document, which this stack shares with the code that has not moved yet.
	SettingsKey = "netaudit"
	// RoutePath is the DNS check's one route, read and written.
	RoutePath = "/admin/api/netaudit/dns-check"
)

// Stack is the Network stack.
type Stack struct {
	mu        sync.Mutex
	cfg       dnscheck.Config
	collector *dnscheck.Collector
	cancel    context.CancelFunc
	done      chan struct{}
}

// New returns the stack.
func New() *Stack { return &Stack{} }

// Manifest declares the stack as data.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: ID, Title: "Network", Purpose: "Watch this network, and what it says to the world.",
		Layers: []stacksdk.Layer{{ID: "outside", Title: "What the internet sees", Purpose: "Your network from the outside"}},
		Modules: []stacksdk.ModuleSpec{{
			ID: ModuleID, Title: "Router DNS check", Purpose: "Is the router answering honestly?", Layer: "outside",
			Sends: stacksdk.SendsInternet, StepUp: true, Monitors: []string{"dns"},
			About: stacksdk.About{
				Watches:  "A few names, asked of your router's DNS and of an outside resolver, to see if the answers agree.",
				Never:    "Raises a finding or changes anything: it only logs what it saw.",
				ShowsUp:  "The summary on this card.",
				Settings: true,
			},
		}},
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RoutePath},
			{Method: http.MethodPut, Path: RoutePath, Access: stacksdk.AccessStepUp},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Storage:  []string{dnscheck.LogFile},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortGuard, stacksdk.PortFetch, stacksdk.PortGateway},
	}
}

// Start builds the collector over the host's ports and runs it under ctx until ctx ends or Stop is called.
func (s *Stack) Start(ctx context.Context, h stacksdk.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("the network stack is already running")
	}
	s.cfg = dnscheck.Config{Doc: h.Config}
	server := func(ctx context.Context) netip.Addr { return routerAddress(ctx, s.cfg, h.Gateway) }
	s.collector = dnscheck.NewCollector(dnscheck.CollectorDeps{
		Settings: func() dnscheck.Settings { return s.cfg.Settings(context.Background()) },
		Router:   dnscheck.Router{Guard: h.Guard, Server: server},
		Outside:  dnscheck.Outside{Fetch: h.Fetch, URL: func() string { return s.cfg.ResolverURL(context.Background()) }},
		Store:    dnscheck.FileStore{Dir: h.DataDir},
		OnError:  func(err error) { h.Log.Log(stacksdk.LevelWarn, "network dns check", "error", err.Error()) },
	})
	run, cancel := context.WithCancel(ctx)
	s.cancel, s.done = cancel, make(chan struct{})
	go func(c *dnscheck.Collector, done chan struct{}) {
		defer close(done)
		c.Run(run)
	}(s.collector, s.done)
	return nil
}

// routerAddress is the router's DNS server: the one the operator set, or else the default gateway, read each time. The zero address means
// neither is known.
func routerAddress(ctx context.Context, cfg dnscheck.Config, gw stacksdk.Gateway) netip.Addr {
	if set, err := netip.ParseAddr(cfg.Server(ctx)); err == nil {
		return set
	}
	if text, err := gw.Default(ctx); err == nil {
		if a, err := netip.ParseAddr(text); err == nil {
			return a
		}
	}
	return netip.Addr{}
}

// Stop ends the collector's loop and waits for it.
func (s *Stack) Stop(context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}

// ConfigChanged has nothing to re-read: every cycle and every request reads the settings document afresh.
func (s *Stack) ConfigChanged(context.Context) error { return nil }

// Status reports the DNS check's latest cycle.
func (s *Stack) Status(ctx context.Context) ([]stacksdk.ModuleStatus, error) {
	s.mu.Lock()
	cfg, c := s.cfg, s.collector
	s.mu.Unlock()
	if c == nil {
		return nil, nil
	}
	set, st := cfg.Settings(ctx), c.Status()
	return []stacksdk.ModuleStatus{{
		ID: ModuleID, Enabled: set.Enabled && len(set.Names) > 0, Every: set.Every, LastRun: st.LastRun, Problem: st.Problem,
	}}, nil
}

// CallTool: the stack declares no tools yet.
func (s *Stack) CallTool(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
}

// response is what GET answers: the settings, the latest cycle and the summary of the log.
type response struct {
	dnscheck.View
	Status  dnscheck.Status  `json:"status"`
	Summary dnscheck.Summary `json:"summary"`
}

type request struct {
	Enabled         *bool     `json:"enabled"`
	IntervalMinutes *int      `json:"interval_minutes"`
	Names           *[]string `json:"names"`
	ResolverURL     *string   `json:"resolver_url"`
	Server          *string   `json:"server"`
}

// Handle answers the DNS check's route.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if req.Path != RoutePath {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	s.mu.Lock()
	cfg, c := s.cfg, s.collector
	s.mu.Unlock()
	if c == nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the network stack is not running")
	}
	switch req.Method {
	case http.MethodGet:
		return get(ctx, cfg, c), nil
	case http.MethodPut:
		return put(ctx, cfg, c, req), nil
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

func get(ctx context.Context, cfg dnscheck.Config, c *dnscheck.Collector) stacksdk.Response {
	summary, err := c.Summary()
	if err != nil {
		return failure(http.StatusInternalServerError, err.Error())
	}
	view, err := cfg.View(ctx)
	if err != nil {
		return failure(http.StatusInternalServerError, err.Error())
	}
	return ok(response{View: view, Status: c.Status(), Summary: summary})
}

// put changes what is sent: the names and resolver first, then the interval, and the switch last, so a refused setting turns nothing on.
// Switching on lets the outside resolver learn the names asked and the network's public address, so the route is a step-up one.
func put(ctx context.Context, cfg dnscheck.Config, c *dnscheck.Collector, req stacksdk.Request) stacksdk.Response {
	if req.ContentType != "application/json" {
		return failure(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
	}
	var in request
	if err := json.Unmarshal(req.Body, &in); err != nil {
		return failure(http.StatusBadRequest, "invalid json: "+err.Error())
	}
	var steps []func() error
	if in.Names != nil {
		steps = append(steps, func() error { return cfg.SetNames(ctx, *in.Names) })
	}
	if in.ResolverURL != nil {
		steps = append(steps, func() error { return cfg.SetResolverURL(ctx, *in.ResolverURL) })
	}
	if in.Server != nil {
		steps = append(steps, func() error { return cfg.SetServer(ctx, *in.Server) })
	}
	if in.IntervalMinutes != nil {
		steps = append(steps, func() error { return cfg.SetInterval(ctx, *in.IntervalMinutes) })
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return failure(http.StatusBadRequest, err.Error())
		}
	}
	if in.Enabled != nil {
		if err := cfg.SetEnabled(ctx, *in.Enabled); err != nil {
			return failure(http.StatusInternalServerError, err.Error())
		}
	}
	return get(ctx, cfg, c)
}

func ok(v any) stacksdk.Response {
	body, err := json.Marshal(v)
	if err != nil {
		return failure(http.StatusInternalServerError, "the answer could not be written")
	}
	return stacksdk.Response{Status: http.StatusOK, ContentType: "application/json", Body: append(body, '\n')}
}

func failure(status int, msg string) stacksdk.Response {
	body, _ := json.Marshal(map[string]string{"error": strings.TrimSpace(msg)})
	return stacksdk.Response{Status: status, ContentType: "application/json", Body: append(body, '\n')}
}
