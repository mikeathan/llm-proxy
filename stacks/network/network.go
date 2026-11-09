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
	"time"

	"llm-proxy-stacks/network/dnscheck"
	"llm-proxy-stacks/network/udpchecks"
	"llm-proxy-stacks/sdk"
)

// ID is the stack's id; the settings key and the files are unchanged from before the move.
const (
	ID       = "network"
	ModuleID = "dns_check"
	// UDPModuleID is the nightly UDP checks, a sensor: the core's registry defines the module and its switch, and this stack answers its
	// Look requests (ADR-0038).
	UDPModuleID = "udp_probes"
	// SettingsKey is the core's `netaudit` settings document, which this stack shares with the code that has not moved yet.
	SettingsKey = "netaudit"
	// RoutePath is the DNS check's one route, read and written.
	RoutePath = "/admin/api/netaudit/dns-check"
	// FindingID is the one finding the DNS check offers; the core words it (ADR-0036).
	FindingID = "dns_router_differs"
)

// Stack is the Network stack.
type Stack struct {
	mu        sync.Mutex
	cfg       dnscheck.Config
	collector *dnscheck.Collector
	store     dnscheck.Store
	gateway   stacksdk.Gateway
	scoped    stacksdk.Guard // the dial that applies the declared scan scope; nil when the host gave none
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
			Sends: stacksdk.SendsInternet, StepUp: true, Monitors: []string{"dns"}, Finds: []string{FindingID},
			About: stacksdk.About{
				Watches:  "A few names, asked of your router's DNS and of an outside resolver, to see if the answers agree.",
				Never:    "Changes anything on your router or network: it only compares and logs what it saw.",
				ShowsUp:  "The summary on this card, and a finding in the nightly report once you switch that on.",
				Settings: true,
			},
		}},
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RoutePath},
			{Method: http.MethodPut, Path: RoutePath, Access: stacksdk.AccessStepUp},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Storage:  []string{dnscheck.LogFile},
		Looks:    []string{UDPModuleID},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortGuard, stacksdk.PortFetch, stacksdk.PortGateway, stacksdk.PortScopedGuard},
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
	s.store, s.gateway, s.scoped = dnscheck.FileStore{Dir: h.DataDir}, h.Gateway, h.ScopedGuard
	s.collector = dnscheck.NewCollector(dnscheck.CollectorDeps{
		Settings: func() dnscheck.Settings { return s.cfg.Settings(context.Background()) },
		Router:   dnscheck.Router{Guard: h.Guard, Server: server},
		Outside:  dnscheck.Outside{Fetch: h.Fetch, URL: func() string { return s.cfg.ResolverURL(context.Background()) }},
		Store:    s.store,
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

// Findings offers the finding for names the router answers differently from an outside resolver on several checks in a row. It is
// pulled by the core when the nightly report is built, reads only the log the check already keeps, and sends nothing. It offers nothing
// until the operator switches the finding on, while the check is off, or when no router address is known to be the subject.
func (s *Stack) Findings(ctx context.Context) ([]stacksdk.StackFinding, error) {
	s.mu.Lock()
	cfg, store, gw := s.cfg, s.store, s.gateway
	s.mu.Unlock()
	if store == nil {
		return nil, nil
	}
	view, err := cfg.View(ctx)
	if err != nil || !view.FindingEnabled || !view.Enabled {
		return nil, err
	}
	router := routerAddress(ctx, cfg, gw)
	if !router.IsValid() {
		return nil, nil
	}
	log, err := store.Read()
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "the DNS check's log could not be read")
	}
	d := dnscheck.Persistent(log, time.Now())
	if len(d.Names) == 0 {
		return nil, nil
	}
	return []stacksdk.StackFinding{{ID: FindingID, Subject: router, Count: d.Count, Names: d.Names, Since: d.Since}}, nil
}

// Look asks the devices the nightly scan found the two fixed UDP questions and says which it asked and which answered in the expected form
// (ADR-0038). Every datagram goes through the scoped dial, which refuses an address outside the operator's declared scan scope; a refusal, an
// error and silence are all "no answer", and the caller treats an asked pair with no answer as unknown, never as fixed. Only IPv4 devices are
// asked, the first udpchecks.MaxDevices of them. Nothing a device said is kept or returned.
func (s *Stack) Look(ctx context.Context, req stacksdk.LookRequest) (stacksdk.LookResult, error) {
	s.mu.Lock()
	scoped := s.scoped
	s.mu.Unlock()
	switch {
	case req.Module != UDPModuleID:
		return stacksdk.LookResult{}, stacksdk.Errorf(stacksdk.CodeInvalid, "this stack has no such sensor")
	case len(req.Addrs) > stacksdk.MaxLookAddrs:
		return stacksdk.LookResult{}, stacksdk.Errorf(stacksdk.CodeInvalid, "too many addresses")
	case scoped == nil:
		return stacksdk.LookResult{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the stack has no scoped dial")
	}
	addrs := make([]netip.Addr, 0, len(req.Addrs))
	for _, a := range req.Addrs {
		if a.Is4() && len(addrs) < udpchecks.MaxDevices {
			addrs = append(addrs, a)
		}
	}
	out := stacksdk.LookResult{Asked: []stacksdk.Pair{}, Answered: []stacksdk.Pair{}}
	for _, a := range addrs {
		for _, p := range udpchecks.Probes {
			out.Asked = append(out.Asked, stacksdk.Pair{Addr: a, Port: p.Port})
		}
	}
	for _, a := range (udpchecks.Prober{Dial: scoped.DialContext}).Look(ctx, addrs) {
		out.Answered = append(out.Answered, stacksdk.Pair{Addr: a.Addr, Port: a.Port})
	}
	return out, nil
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
	FindingEnabled  *bool     `json:"finding_enabled"`
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
	if in.FindingEnabled != nil {
		steps = append(steps, func() error { return cfg.SetFindingEnabled(ctx, *in.FindingEnabled) })
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
