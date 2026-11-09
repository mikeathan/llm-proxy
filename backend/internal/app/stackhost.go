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
