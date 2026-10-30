package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nmap"
	"llm-proxy/internal/core/netaudit/portprobe"
	"llm-proxy/internal/core/netaudit/scantool"
	"llm-proxy/internal/core/netaudit/upnp"
	"llm-proxy/internal/core/netaudit/vulnfeed"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/neighbors"
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
	// Expectations holds what the operator says each device should offer.
	Expectations *netaudit.Expectations
	// Deep runs the operator-started deep check of one device and keeps its result.
	Deep *netaudit.DeepChecks
	// Vulns holds the vulnerability data and runs its operator-started download.
	Vulns *vulnfeed.Manager
	// Inventory is who has been seen on the network, shared by the nightly audit and the presence check.
	Inventory *netaudit.Inventory
	// Identities remembers each service's SSH host keys and TLS certificate, to report one that was replaced.
	Identities *netaudit.Identities
	// Router asks the router whether it offers UPnP, in the nightly audit only.
	Router *netaudit.RouterCheck
	// PresenceConfig holds the presence check's switch and interval; Presence is the check itself, started with the app.
	PresenceConfig *netaudit.PresenceConfig
	Presence       *netaudit.Presence
}

// neighborSource adapts the host's neighbour table to the audit's port.
type neighborSource struct{ reader neighbors.Reader }

func (n neighborSource) Neighbors(context.Context) (netaudit.NeighborTable, error) {
	table, err := n.reader.Read()
	if err != nil {
		return netaudit.NeighborTable{}, err
	}
	out := netaudit.NeighborTable{Gateway: table.Gateway, Entries: make([]netaudit.Neighbor, 0, len(table.Entries))}
	for _, e := range table.Entries {
		out.Entries = append(out.Entries, netaudit.Neighbor{Addr: e.Addr, MAC: e.MAC})
	}
	return out, nil
}

// NetauditInventory is where the run reporter looks at who is on the network; nil until the services are wired.
func (s AppServices) NetauditInventory() netaudit.InventoryLook {
	if s.netaudit == nil || s.netaudit.Inventory == nil {
		return nil
	}
	return s.netaudit.Inventory
}

// NetauditIdentities is where the run reporter compares host keys and certificates; nil until the services are wired.
func (s AppServices) NetauditIdentities() netaudit.IdentityTracker {
	if s.netaudit == nil || s.netaudit.Identities == nil {
		return nil
	}
	return s.netaudit.Identities
}

// NetauditRouter is where the run reporter asks the router about UPnP; nil until the services are wired.
func (s AppServices) NetauditRouter() netaudit.RouterFindings {
	if s.netaudit == nil || s.netaudit.Router == nil {
		return nil
	}
	return s.netaudit.Router
}

// StartNetauditPresence runs the presence check until ctx ends. It is the one place that starts it (Constitution II.2,
// II.14); a check that is switched off in the settings does nothing.
func (s AppServices) StartNetauditPresence(ctx context.Context) {
	if s.netaudit != nil && s.netaudit.Presence != nil {
		go s.netaudit.Presence.Run(ctx)
	}
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
	inventoryStore := persistence.NewDeviceInventoryStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The list of devices seen on the network", inventoryStore.Delete)
	inventory := netaudit.NewInventory(inventoryStore, neighborSource{reader: neighbors.NewReader()},
		func(a netip.Addr) bool { return scope.Policy().Check(a) == nil })
	identityStore := persistence.NewDeviceIdentityStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The remembered SSH host keys and certificates", identityStore.Delete)
	presenceConfig := netaudit.NewPresenceConfig(store)
	s.netaudit = &netauditServices{Router: newRouterCheck(netTools, inventory, scope), Inventory: inventory, Identities: netaudit.NewIdentities(identityStore), PresenceConfig: presenceConfig, Presence: s.newPresence(service, inventory, presenceConfig), Scope: scope, Readiness: readiness, Acks: netaudit.NewAcks(s.Persistence(), s.Persistence()), Fixes: netaudit.NewFixes(s.Persistence(), s.Persistence()), Expectations: netaudit.NewExpectations(s.Persistence(), scope), Deep: netaudit.NewDeepChecks(service, s.Persistence(), s.Persistence()), Vulns: vulns}

	if registry := localRegistry(s.toolProvider); registry != nil {
		registry.Use(scantool.Module(service, s.Persistence(), func(context.Context) bool { return len(readiness.Blocking()) == 0 }))
	}
}

// newPresence builds the presence check. It sweeps through the scan service, so the declared scope and the one-scan-at-a-time
// rule apply; the alert goes to every connector, in counts and fixed words, and no model is involved.
func (s *AppServices) newPresence(service *netaudit.Service, inventory *netaudit.Inventory, config *netaudit.PresenceConfig) *netaudit.Presence {
	return netaudit.NewPresence(netaudit.PresenceDeps{
		Settings: config.Settings,
		Sweep: func(ctx context.Context, targets []string) error {
			_, err := service.Scan(ctx, netaudit.Request{Targets: targets, Mode: netaudit.ModeDiscover, Unattended: true})
			return err
		},
		Inventory: inventory,
		Notify: func(ctx context.Context, message string) error {
			comm := communicationTools(s.toolProvider)
			if comm == nil || len(comm.Types()) == 0 {
				// Not marked as told: the alert stays pending until a connector exists, and the screen shows it meanwhile.
				return errors.New("no connector is set up to send alerts")
			}
			return comm.NotifyAll(ctx, message, "")
		},
		OnError: func(err error) { logging.Warn("network presence check", "error", err.Error()) },
	})
}

// netauditPresence joins the presence check's settings with the check's own status for the handlers.
type netauditPresence struct {
	*netaudit.PresenceConfig
	check *netaudit.Presence
}

func (p netauditPresence) Status() netaudit.PresenceStatus { return p.check.Status() }

// newRouterCheck builds the UPnP check of the router. Both the datagram and the page fetch go through the guarded network
// client, so the global network guardrails apply, and the router must be inside the declared scan scope.
func newRouterCheck(netTools *tools.NetworkTools, inventory *netaudit.Inventory, scope *netaudit.Scope) *netaudit.RouterCheck {
	client := upnp.Client{Dial: netTools.DialContext(), HTTP: netTools.HTTPClient()}
	gateway := func() (netip.Addr, error) {
		inv, err := inventory.View()
		if err != nil || inv.Gateway == "" {
			return netip.Addr{}, err
		}
		return netip.ParseAddr(inv.Gateway)
	}
	return netaudit.NewRouterCheck(client, gateway, func(a netip.Addr) error { return scope.Policy().Check(a) })
}
