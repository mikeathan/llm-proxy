package app

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nmap"
	"llm-proxy/internal/core/netaudit/portprobe"
	"llm-proxy/internal/core/netaudit/scantool"
	"llm-proxy/internal/core/netaudit/vulnfeed"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/progexec"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// netauditServices are the network-audit pieces the app layer shares: the HTTP handlers read the scope and the
// readiness, the agent's network_scan tool uses the service.
type netauditServices struct {
	Scope     *netaudit.Scope
	Readiness *netaudit.Readiness
	Acks      *netaudit.Acks
	Fixes     *netaudit.Fixes
	// Vulns holds the vulnerability data and runs its operator-started download.
	Vulns *vulnfeed.Manager
}

// NetauditVulns is where the run reporter reads the vulnerability data from; nil until the services are wired.
func (s AppServices) NetauditVulns() netaudit.VulnSource {
	if s.netaudit == nil || s.netaudit.Vulns == nil {
		return nil
	}
	return s.netaudit.Vulns.Index
}

// NetauditPreflight is the check the dispatcher runs before a scheduled audit touches the model; it is nil until the
// network-audit services are wired.
func (s AppServices) NetauditPreflight() automation.Preflight {
	if s.netaudit == nil {
		return nil
	}
	readiness := s.netaudit.Readiness
	return func(ctx context.Context, entry *automation.AutomationEntry) error {
		if code, message, refused := readiness.Preflight(ctx, entry.Profile); refused {
			return &automation.PreflightError{Code: code, Message: message}
		}
		return s.auditModelIsLocal(entry)
	}
}

// ModelNotLocalCode is the pre-flight reason for an audit whose model runs in the cloud.
const ModelNotLocalCode = "model_not_local"

// auditModelIsLocal refuses a scheduled audit whose model is not local, before any model is woken: the auditor's findings
// must not leave the machine, and a cloud run would stop on that rule after already contacting its provider. A model
// that cannot be found here is left to fail in the run itself.
func (s AppServices) auditModelIsLocal(entry *automation.AutomationEntry) error {
	if entry.Profile != netaudit.AuditorProfileID {
		return nil
	}
	name := entry.Model
	if name == "" {
		name, _ = s.SelectModels()
	}
	cfg, known := s.ModelConfig(name)
	if !known || cfg.WorkloadClass == models.WorkloadLocal {
		return nil
	}
	return &automation.PreflightError{
		Code:    ModelNotLocalCode,
		Message: fmt.Sprintf("The model %q runs in the cloud, and a network audit may only use a model on this machine. Choose a local model for the automation.", name),
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
		netaudit.PolicyAuthorizer{Policy: scope.Policy, Resolver: net.DefaultResolver}, nmapEngine, probe).WithDeviceTimeout(scope.DeviceTimeout)

	// The vulnerability data is downloaded only when the operator asks. The fetch goes through the guarded network
	// client, so the global network guardrails and the blocked-domain list apply to it like to any agent fetch.
	netTools := tools.NewNetworkTools(func(context.Context) models.NetworkGuardrailsConfig {
		return dataMgr.Settings().Get().Guardrails.Network
	}, s.logger)
	vulns := vulnfeed.New(persistence.NewVulnIndexStore(s.AppCtx.RootDir()), netTools.Open, vulnfeed.DefaultSources())

	readiness := netaudit.NewReadiness(netaudit.ReadinessDeps{
		Config:        store.Get,
		Nmap:          nmapEngine,
		Sandbox:       s.AppCtx.sandboxProv,
		HostNetworkOn: func() bool { return s.AppCtx.HostSettings().Sandboxing.NetworkAllowed() },
		VulnData: func() netaudit.VulnDataView {
			st := vulns.Status()
			return netaudit.VulnDataView{Running: st.State == vulnfeed.StateRunning, FilesDone: st.FilesDone, FilesTotal: st.FilesTotal, Age: st.Age(time.Now())}
		},
	})
	// The downloaded vulnerability data lives beside the settings, outside what a factory reset replaces; a reset clears it too.
	s.AppCtx.AfterFactoryReset("The downloaded vulnerability data", vulns.Clear)
	s.netaudit = &netauditServices{Scope: scope, Readiness: readiness, Acks: netaudit.NewAcks(s.Persistence(), s.Persistence()), Fixes: netaudit.NewFixes(s.Persistence(), s.Persistence()), Vulns: vulns}

	if registry := localRegistry(s.toolProvider); registry != nil {
		registry.Use(scantool.Module(service, s.Persistence(), func(context.Context) bool { return len(readiness.Blocking()) == 0 }))
	}
}
