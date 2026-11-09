package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit"
)

// stackHost is one stack running inside the service: the stack and the ports it was started with. It is the
// one place that starts and stops a stack, so a stack's background work is never started ad hoc. It names no stack: the composition root
// (stacks.go) is the only file that imports one.
type stackHost struct {
	stack stacksdk.Stack
	host  stacksdk.Host
}

// newStackHost checks the manifest and that the host gives every port the manifest requires (and none it does not).
func newStackHost(s stacksdk.Stack, h stacksdk.Host) (*stackHost, error) {
	m := s.Manifest()
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("failed to host stack %q: %w", m.ID, err)
	}
	if err := checkPorts(m, h); err != nil {
		return nil, err
	}
	return &stackHost{stack: s, host: h}, nil
}

// checkPorts holds the host to the manifest: a stack gets exactly the ports it declared.
func checkPorts(m stacksdk.Manifest, h stacksdk.Host) error {
	given := map[string]bool{
		stacksdk.PortConfig: h.Config != nil, stacksdk.PortEvents: h.Events != nil, stacksdk.PortLog: h.Log != nil,
		stacksdk.PortDataDir: h.DataDir != nil, stacksdk.PortGuard: h.Guard != nil, stacksdk.PortFetch: h.Fetch != nil,
		stacksdk.PortGateway: h.Gateway != nil, stacksdk.PortIsolation: h.Isolation != nil, stacksdk.PortPrivileged: h.Privileged != nil,
		stacksdk.PortScopedGuard: h.ScopedGuard != nil,
	}
	for port, has := range given {
		if want := slices.Contains(m.Requires, port); want != has {
			return fmt.Errorf("stack %q: the %q port is %s but the manifest %s it", m.ID, port, map[bool]string{true: "given", false: "missing"}[has], map[bool]string{true: "does not require", false: "requires"}[has])
		}
	}
	return nil
}

// Run starts the stack under ctx and arranges for it to stop when ctx ends. It returns at once and holds no goroutine of its own: the stack
// runs its own background work, so hosting adds none. It is one of the Network module's background loops.
func (h *stackHost) Run(ctx context.Context) {
	if err := h.stack.Start(ctx, h.host); err != nil {
		h.host.Log.Log(stacksdk.LevelError, "the stack did not start", "stack", h.stack.Manifest().ID, "error", err.Error())
		return
	}
	context.AfterFunc(ctx, func() { _ = h.stack.Stop(context.Background()) })
}

// Module returns the status of one module of the stack, if the stack reports it.
func (h *stackHost) Module(ctx context.Context, id string) (stacksdk.ModuleStatus, bool) {
	list, err := h.stack.Status(ctx)
	if err != nil {
		return stacksdk.ModuleStatus{}, false
	}
	for _, st := range list {
		if st.ID == id {
			return st, true
		}
	}
	return stacksdk.ModuleStatus{}, false
}

// claimedFiles checks that no two stacks claim one file and returns every claimed name. A core store's file is never claimed by a stack:
// internal/archtest checks the names a stack lists against the core's source.
func claimedFiles(manifests ...stacksdk.Manifest) ([]string, error) {
	owner := map[string]string{}
	var all []string
	for _, m := range manifests {
		for _, name := range m.Storage {
			if other, taken := owner[name]; taken {
				return nil, fmt.Errorf("stacks %q and %q both claim the file %q", other, m.ID, name)
			}
			owner[name] = m.ID
			all = append(all, name)
		}
	}
	return all, nil
}

// offerTimeout bounds how long the nightly report waits for a stack's findings; a variable so a test can shorten it.
var offerTimeout = 5 * time.Second

// Offered pulls the findings the stack's modules offer, for the nightly report (ADR-0036). It keeps only ids a module declared in its
// manifest, in the core's own type, and gives up after offerTimeout: a stack that is slow or fails adds nothing and the report says so.
// The core words and checks each one (netaudit.StackFindings); this only holds the stack to what it announced.
func (h *stackHost) Offered(ctx context.Context) ([]netaudit.OfferedFinding, error) {
	ctx, cancel := context.WithTimeout(ctx, offerTimeout)
	defer cancel()
	got, err := h.stack.Findings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the findings of stack %q: %w", h.stack.Manifest().ID, err)
	}
	declared := map[string]bool{}
	for _, m := range h.stack.Manifest().Modules {
		for _, id := range m.Finds {
			declared[id] = true
		}
	}
	var out []netaudit.OfferedFinding
	for _, f := range got {
		if declared[f.ID] {
			out = append(out, netaudit.OfferedFinding{ID: f.ID, Subject: f.Subject, Count: f.Count, Names: slices.Clone(f.Names), Since: f.Since})
		}
	}
	return out, nil
}

// lookTimeout bounds how long the nightly report waits for a sensor: 64 devices, two questions each, four at a time, two seconds a question is
// about a minute at worst. A variable so a test can shorten it.
var lookTimeout = 3 * time.Minute

// Look asks a sensor module of the stack to look at the given addresses (ADR-0038). It holds the stack to what its manifest announced (the
// module must be listed in Looks, the stack must implement Looker) and to the bound on addresses, and gives up after lookTimeout: a sensor
// that is slow or fails finds nothing and the caller says it could not look. What the stack sends goes through its scoped dial only.
func (h *stackHost) Look(ctx context.Context, module string, addrs []netip.Addr) (stacksdk.LookResult, error) {
	id := h.stack.Manifest().ID
	looker, ok := h.stack.(stacksdk.Looker)
	switch {
	case !ok || !slices.Contains(h.stack.Manifest().Looks, module):
		return stacksdk.LookResult{}, fmt.Errorf("stack %q has no sensor %q", id, module)
	case len(addrs) > stacksdk.MaxLookAddrs:
		return stacksdk.LookResult{}, fmt.Errorf("too many addresses for stack %q to look at: %d", id, len(addrs))
	}
	ctx, cancel := context.WithTimeout(ctx, lookTimeout)
	defer cancel()
	got, err := looker.Look(ctx, stacksdk.LookRequest{Module: module, Addrs: slices.Clone(addrs)})
	if err != nil {
		return stacksdk.LookResult{}, fmt.Errorf("stack %q could not look: %w", id, err)
	}
	return got, nil
}

// stackUDPAsker is the nightly UDP checks' asker: the Network stack's sensor, reached through its host, in the core's own types.
type stackUDPAsker struct{ host *stackHost }

// Look asks the UDP sensor about the addresses. A stack that was not hosted cannot look, and the checks then keep every earlier finding.
func (a stackUDPAsker) Look(ctx context.Context, addrs []netip.Addr) (netaudit.UDPLooked, error) {
	if a.host == nil {
		return netaudit.UDPLooked{}, errors.New("the Network stack is not hosted")
	}
	got, err := a.host.Look(ctx, networkUDPModule, addrs)
	if err != nil {
		return netaudit.UDPLooked{}, err
	}
	return netaudit.UDPLooked{Asked: udpPairs(got.Asked), Answered: udpPairs(got.Answered)}, nil
}

func udpPairs(in []stacksdk.Pair) []netaudit.UDPPair {
	out := make([]netaudit.UDPPair, 0, len(in))
	for _, p := range in {
		out = append(out, netaudit.UDPPair{Addr: p.Addr, Port: p.Port})
	}
	return out
}

// webLookGrace is how long past a run's own time limit the host waits for a web sensor to answer: the stack ends its run at the limit and
// returns what it found, so only the hand-over needs time. A variable so a test can shorten it.
var webLookGrace = 30 * time.Second

// webSensor holds a web request to what the manifest announced (the module is listed in Looks and the stack implements WebLooker) and to the
// bounds, and returns the stack's sensor and a context that ends a short grace after the run's own limit. The nightly run of the web checks is
// allowed ten minutes, so the wait follows the request, not the fixed time of the UDP look.
func (h *stackHost) webSensor(ctx context.Context, req stacksdk.WebRequest) (stacksdk.WebLooker, context.Context, context.CancelFunc, error) {
	id := h.stack.Manifest().ID
	sensor, ok := h.stack.(stacksdk.WebLooker)
	switch {
	case !ok || !slices.Contains(h.stack.Manifest().Looks, req.Module):
		return nil, nil, nil, fmt.Errorf("stack %q has no web sensor %q", id, req.Module)
	case len(req.Targets) < 1 || len(req.Targets) > stacksdk.MaxWebTargets:
		return nil, nil, nil, fmt.Errorf("stack %q was asked to look at %d web services", id, len(req.Targets))
	case req.LimitSeconds < 1 || req.LimitSeconds > stacksdk.MaxLookSeconds:
		return nil, nil, nil, fmt.Errorf("stack %q was given a time limit of %d seconds", id, req.LimitSeconds)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(req.LimitSeconds)*time.Second+webLookGrace)
	return sensor, ctx, cancel, nil
}

// LookWeb asks the stack's web sensor to make its plain requests of the named services.
func (h *stackHost) LookWeb(ctx context.Context, req stacksdk.WebRequest) (stacksdk.WebResult, error) {
	sensor, ctx, cancel, err := h.webSensor(ctx, req)
	if err != nil {
		return stacksdk.WebResult{}, err
	}
	defer cancel()
	got, err := sensor.LookWeb(ctx, req)
	if err != nil {
		return stacksdk.WebResult{}, fmt.Errorf("stack %q could not look at the web services: %w", h.stack.Manifest().ID, err)
	}
	return got, nil
}

// LookCertificates asks the stack's web sensor for the handshakes with the secure services among the named ones.
func (h *stackHost) LookCertificates(ctx context.Context, req stacksdk.WebRequest) (stacksdk.CertResult, error) {
	sensor, ctx, cancel, err := h.webSensor(ctx, req)
	if err != nil {
		return stacksdk.CertResult{}, err
	}
	defer cancel()
	got, err := sensor.LookCertificates(ctx, req)
	if err != nil {
		return stacksdk.CertResult{}, fmt.Errorf("stack %q could not read the certificates: %w", h.stack.Manifest().ID, err)
	}
	return got, nil
}

// stackWebChecker is the web checks' checker: the Network stack's web sensor, reached through its host, in the core's own types. Before it
// asks, it makes the checks the prober used to make for itself, so a run the operator's settings forbid is an error the report states and not
// a silent "nothing found": every target must be an IPv4 address inside the declared scan scope, and the guardrails must allow the local
// network. (The stack's scoped dial enforces the scope again at every connection.)
type stackWebChecker struct {
	host   *stackHost
	scope  func(netip.Addr) error // the declared scan scope
	ready  func() (bool, string)  // whether the guardrails allow the local network, and the words for why not
	module string
}

// request checks a run and builds the stack's request. what names the check in an error.
func (c stackWebChecker) request(what string, targets []netaudit.WebTarget, limit time.Duration) (stacksdk.WebRequest, error) {
	switch {
	case len(targets) == 0 || len(targets) > netaudit.MaxWebTargets:
		return stacksdk.WebRequest{}, fmt.Errorf("%s: %d targets, want 1 to %d", what, len(targets), netaudit.MaxWebTargets)
	case limit < time.Second:
		return stacksdk.WebRequest{}, fmt.Errorf("%s: a run needs a time limit of at least a second", what)
	}
	if ok, why := c.ready(); !ok {
		return stacksdk.WebRequest{}, fmt.Errorf("%s: %s", what, why)
	}
	out := stacksdk.WebRequest{Module: c.module, LimitSeconds: int(min(limit.Seconds(), stacksdk.MaxLookSeconds)), Targets: make([]stacksdk.WebTarget, 0, len(targets))}
	for _, t := range targets {
		if !t.Addr.Is4() || t.Port < 1 || t.Port > 65535 {
			return stacksdk.WebRequest{}, fmt.Errorf("%s: %s:%d is not an IPv4 address and port", what, t.Addr, t.Port)
		}
		if err := c.scope(t.Addr); err != nil {
			return stacksdk.WebRequest{}, fmt.Errorf("%s: %s is outside the scan scope: %w", what, t.Addr, err)
		}
		out.Targets = append(out.Targets, stacksdk.WebTarget{Addr: t.Addr, Port: t.Port, TLS: t.TLS})
	}
	return out, nil
}

var errWebNotHosted = errors.New("the Network stack is not hosted")

// Probe makes the plain web checks.
func (c stackWebChecker) Probe(ctx context.Context, targets []netaudit.WebTarget, limit time.Duration) (netaudit.WebResult, error) {
	req, err := c.request("web checks", targets, limit)
	if err != nil {
		return netaudit.WebResult{}, err
	}
	if c.host == nil {
		return netaudit.WebResult{}, errWebNotHosted
	}
	got, err := c.host.LookWeb(ctx, req)
	if err != nil {
		return netaudit.WebResult{}, err
	}
	out := netaudit.WebResult{Hits: make([]netaudit.WebHit, 0, len(got.Hits)), Partial: got.Partial}
	for _, h := range got.Hits {
		out.Hits = append(out.Hits, netaudit.WebHit{Addr: h.Addr, Port: h.Port, ID: h.ID})
	}
	return out, nil
}

// ProbeTLS makes the certificate handshakes.
func (c stackWebChecker) ProbeTLS(ctx context.Context, targets []netaudit.WebTarget, limit time.Duration) (netaudit.TLSResult, error) {
	req, err := c.request("certificate checks", targets, limit)
	if err != nil {
		return netaudit.TLSResult{}, err
	}
	if c.host == nil {
		return netaudit.TLSResult{}, errWebNotHosted
	}
	got, err := c.host.LookCertificates(ctx, req)
	if err != nil {
		return netaudit.TLSResult{}, err
	}
	out := netaudit.TLSResult{Facts: make([]netaudit.TLSFact, 0, len(got.Facts)), Partial: got.Partial}
	for _, f := range got.Facts {
		out.Facts = append(out.Facts, netaudit.TLSFact{Addr: f.Addr, Port: f.Port, NotBefore: f.NotBefore, NotAfter: f.NotAfter, SelfSigned: f.SelfSigned, OldVersions: slices.Clone(f.OldVersions)})
	}
	return out, nil
}
