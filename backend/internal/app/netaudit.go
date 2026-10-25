package app

import (
	"context"
	"net"
	"net/netip"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nmap"
	"llm-proxy/internal/core/netaudit/portprobe"
	"llm-proxy/internal/core/netaudit/scantool"
	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/storage"
)

// netauditServices are the network-audit pieces the app layer shares: the HTTP handlers read the scope and the
// readiness, the agent's network_scan tool uses the service.
type netauditServices struct {
	Scope     *netaudit.Scope
	Readiness *netaudit.Readiness
	Acks      *netaudit.Acks
}

// NetauditPreflight is the check the dispatcher runs before a scheduled audit touches the model; it is nil until the
// network-audit services are wired.
func (s AppServices) NetauditPreflight() automation.Preflight {
	if s.netaudit == nil {
		return nil
	}
	readiness := s.netaudit.Readiness
	return func(ctx context.Context, entry *automation.AutomationEntry) error {
		code, message, refused := readiness.Preflight(ctx, entry.Profile)
		if !refused {
			return nil
		}
		return &automation.PreflightError{Code: code, Message: message}
	}
}

// wireNetaudit builds the network-audit services and registers the network_scan tool. It is the one place they are
// put together: the policy comes from settings, the engines from the sandbox provider, and the tool is hidden
// whenever the readiness check says a scan cannot run.
func (s *AppServices) wireNetaudit(dataMgr *storage.DataManager) {
	store := dataMgr.Netaudit()
	scope := netaudit.NewScope(store, net.DefaultResolver, nil)

	runner := progexec.Runner{Sandbox: s.AppCtx.sandboxProv, AllowUnconfined: func() bool { return store.Get().AllowUnconfined }}
	nmapEngine := nmap.New(runner)
	probe := portprobe.New(func(a netip.Addr) error { return scope.Policy().Check(a) })
	service := netaudit.NewService(
		netaudit.PolicyAuthorizer{Policy: scope.Policy, Resolver: net.DefaultResolver}, nmapEngine, probe)

	readiness := netaudit.NewReadiness(netaudit.ReadinessDeps{
		Config:        store.Get,
		Nmap:          nmapEngine,
		Sandbox:       s.AppCtx.sandboxProv,
		HostNetworkOn: func() bool { return s.AppCtx.HostSettings().Sandboxing.NetworkAllowed() },
	})
	s.netaudit = &netauditServices{Scope: scope, Readiness: readiness, Acks: netaudit.NewAcks(s.Persistence(), s.Persistence())}

	if registry := localRegistry(s.toolProvider); registry != nil {
		registry.Use(scantool.Module(service, s.Persistence(), func(context.Context) bool { return len(readiness.Blocking()) == 0 }))
	}
}
