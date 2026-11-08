package app

import (
	"context"
	"fmt"
	"slices"

	"llm-proxy-stacks/sdk"
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
