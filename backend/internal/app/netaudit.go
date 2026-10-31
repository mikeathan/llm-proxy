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
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/names"
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
	// Names asks devices the operator has not named what they call themselves, once per presence look, and keeps the answers
	// for the Devices card to offer.
	Names *names.Suggester
	// NamesConfig is the switch for asking devices their names (on unless switched off).
	NamesConfig *netaudit.NamesConfig
	// Summary is the switch for the weekly summary, which the presence look sends when it is due.
	Summary *netaudit.SummaryConfig
	// History keeps the daily counts the Network page draws its trends from.
	History *netaudit.History
	// Inventory is who has been seen on the network, shared by the nightly audit and the presence check.
	Inventory *netaudit.Inventory
	// Identities remembers each service's SSH host keys and TLS certificate, to report one that was replaced.
	Identities *netaudit.Identities
	// Router asks the router whether it offers UPnP, in the nightly audit only.
	Router *netaudit.RouterCheck
	// DecoyConfig is the decoy services' switch; Decoys runs the listeners and DecoyStore keeps who connected.
	DecoyConfig *netaudit.DecoyConfig
	Decoys      *decoy.Manager
	DecoyStore  *persistence.DecoyTouchStore
	// Alerts is the log of what the Network module alerted about, kept whether or not a connector exists.
	Alerts *netaudit.AlertLog
	// Discovery6 is the switch for active IPv6 discovery, which the inventory runs on each look while it is on.
	Discovery6 *netaudit.Discovery6Config
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
	out := netaudit.NeighborTable{Gateway: table.Gateway, Entries: neighborsOf(table.Entries), Entries6: neighborsOf(table.Entries6)}
	return out, nil
}

// echoDiscoverer adapts the platform's IPv6 echo to the audit's port, turning the platform's errors into the audit's.
type echoDiscoverer struct{}

func (echoDiscoverer) Discover(ctx context.Context, interfaces []string) error {
	err := neighbors.Discover6(ctx, interfaces)
	switch {
	case errors.Is(err, neighbors.ErrNotPermitted):
		return netaudit.ErrDiscoveryNotPermitted
	case errors.Is(err, neighbors.ErrUnavailable):
		return netaudit.ErrDiscoveryUnavailable
	}
	return err
}

func neighborsOf(entries []neighbors.Entry) []netaudit.Neighbor {
	out := make([]netaudit.Neighbor, 0, len(entries))
	for _, e := range entries {
		out = append(out, netaudit.Neighbor{Addr: e.Addr, MAC: e.MAC, Interface: e.Interface})
	}
	return out
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

// NetauditHistory is where the run reporter notes each audit's finding counts; nil until the services are wired.
func (s AppServices) NetauditHistory() netaudit.AuditHistory {
	if s.netaudit == nil || s.netaudit.History == nil {
		return nil
	}
	return s.netaudit.History
}

// NetauditDecoys is where the run reporter reads the decoy touches; nil until the services are wired.
func (s AppServices) NetauditDecoys() netaudit.DecoyReader {
	if s.netaudit == nil || s.netaudit.DecoyStore == nil {
		return nil
	}
	return s.netaudit.DecoyStore
}

// StartNetauditPresence runs the presence check until ctx ends. It is the one place that starts it (Constitution II.2,
// II.14); a check that is switched off in the settings does nothing.
func (s AppServices) StartNetauditPresence(ctx context.Context) {
	if s.netaudit != nil && s.netaudit.Presence != nil {
		go s.netaudit.Presence.Run(ctx)
	}
	if s.netaudit != nil && s.netaudit.Decoys != nil {
		go s.netaudit.Decoys.Run(ctx)
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
	nameStore := persistence.NewNameSuggestionStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The names devices gave when asked", nameStore.Delete)
	namesConfig := netaudit.NewNamesConfig(store)
	suggester := names.NewSuggester(nameStore, scopedAsker(names.Prober{Dial: netTools.DialContext()}.Ask, scope)).WithEnabled(namesConfig.Enabled)
	historyStore := persistence.NewNetworkHistoryStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The daily counts behind the Network page's trends", historyStore.Delete)
	history := netaudit.NewHistory(historyStore)
	summaryConfig := netaudit.NewSummaryConfig(store)
	discovery6 := netaudit.NewDiscovery6Config(store)
	inventory := netaudit.NewInventory(inventoryStore, neighborSource{reader: neighbors.NewReader()},
		func(a netip.Addr) bool { return scope.Policy().Check(a) == nil }).WithDiscovery6(echoDiscoverer{}, discovery6.Enabled).WithObserver(recordLook(history))
	identityStore := persistence.NewDeviceIdentityStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The remembered SSH host keys and certificates", identityStore.Delete)
	presenceConfig := netaudit.NewPresenceConfig(store)
	decoyStore := persistence.NewDecoyTouchStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The record of connections to decoy services", decoyStore.Delete)
	decoyConfig := netaudit.NewDecoyConfig(store)
	alertStore := persistence.NewNetworkAlertStore(s.AppCtx.RootDir())
	s.AppCtx.AfterFactoryReset("The log of the Network module's alerts", alertStore.Delete)
	alerts := netaudit.NewAlertLog(alertStore, time.Now, func(err error) { logging.Warn("network alerts", "error", err.Error()) })
	digest := s.newDigest(inventory, history, summaryConfig, s.alertNotifier(alerts, models.NetworkAlertSummary))
	s.netaudit = &netauditServices{NamesConfig: namesConfig, Summary: summaryConfig, Names: suggester, History: history, Discovery6: discovery6, DecoyConfig: decoyConfig, DecoyStore: decoyStore, Alerts: alerts, Decoys: s.newDecoys(decoyConfig, decoyStore, alerts, scope), Router: newRouterCheck(netTools, inventory, scope), Inventory: inventory, Identities: netaudit.NewIdentities(identityStore), PresenceConfig: presenceConfig, Presence: s.newPresence(service, inventory, presenceConfig, presenceHooks{OnLook: lookHooks(digest, suggester), Notify: s.alertNotifier(alerts, models.NetworkAlertPresence)}), Scope: scope, Readiness: readiness, Acks: netaudit.NewAcks(s.Persistence(), s.Persistence()), Fixes: netaudit.NewFixes(s.Persistence(), s.Persistence()), Expectations: netaudit.NewExpectations(s.Persistence(), scope), Deep: netaudit.NewDeepChecks(service, s.Persistence(), s.Persistence()), Vulns: vulns}

	if registry := localRegistry(s.toolProvider); registry != nil {
		registry.Use(scantool.Module(service, s.Persistence(), func(context.Context) bool { return len(readiness.Blocking()) == 0 }))
	}
}

// recordLook notes each look at the network in the history: how many devices answered, how many are known, how many of
// those have no name. A failure is logged and never stops the look.
func recordLook(history *netaudit.History) func(netaudit.Snapshot) {
	return func(snap netaudit.Snapshot) {
		unnamed := 0
		for _, d := range snap.Inventory.Devices {
			if d.Name == "" {
				unnamed++
			}
		}
		if err := history.RecordLook(len(snap.Current), len(snap.Inventory.Devices), unnamed); err != nil {
			logging.Warn("network history", "error", err.Error())
		}
	}
}

// scopedAsker refuses to ask a device outside the declared scan scope, on top of what the guarded dialer refuses.
func scopedAsker(ask names.Asker, scope *netaudit.Scope) names.Asker {
	return func(ctx context.Context, addr netip.Addr) (string, error) {
		if err := scope.Policy().Check(addr); err != nil {
			return "", err
		}
		return ask(ctx, addr)
	}
}

// nameUnnamed asks the devices the operator has not named what they call themselves, as part of each presence look. The
// questions go through the guarded dialer, so only devices inside the declared scan scope are asked; a failure is logged
// and never stops the look.
func nameUnnamed(suggester *names.Suggester) func(context.Context, netaudit.Snapshot) {
	return func(ctx context.Context, snap netaudit.Snapshot) {
		var candidates []names.Candidate
		for _, d := range snap.Inventory.Devices {
			if addr, err := netip.ParseAddr(d.IP); d.Name == "" && err == nil && addr.Is4() {
				candidates = append(candidates, names.Candidate{MAC: d.MAC, Addr: addr})
			}
		}
		if err := suggester.Look(ctx, candidates); err != nil {
			logging.Warn("network device names", "error", err.Error())
		}
	}
}

// sendToConnectors sends one message to every connector. Without one it fails with netaudit.ErrNoConnector, so whatever was
// to be told stays pending (an alert stays on the screen, a summary stays due) until a connector exists.
func (s *AppServices) sendToConnectors(ctx context.Context, message string) error {
	comm := communicationTools(s.toolProvider)
	if comm == nil || len(comm.Types()) == 0 {
		return netaudit.ErrNoConnector
	}
	return comm.NotifyAll(ctx, message, "")
}

// alertNotifier is how an alert of one kind goes out: into the alert log first, so the web UI shows it whether or not a
// connector exists, then to every connector.
func (s *AppServices) alertNotifier(alerts *netaudit.AlertLog, kind models.NetworkAlertKind) func(ctx context.Context, message string) error {
	return func(ctx context.Context, message string) error {
		return alerts.Send(ctx, kind, message, s.sendToConnectors)
	}
}

// newDigest builds the weekly summary, which the presence look sends when it is due.
func (s *AppServices) newDigest(inventory *netaudit.Inventory, history *netaudit.History, summary *netaudit.SummaryConfig, notify func(context.Context, string) error) *netaudit.WeeklyDigest {
	return netaudit.NewWeeklyDigest(netaudit.DigestDeps{
		Enabled: summary.Enabled, View: inventory.View, History: history, Notify: notify,
		OnError: func(err error) { logging.Warn("network weekly summary", "error", err.Error()) },
	})
}

// lookHooks is the work that rides on each presence look, in one place: asking unnamed devices their names, and sending
// the weekly summary when it is due. Neither adds a sweep; both stop with the context.
func lookHooks(digest *netaudit.WeeklyDigest, suggester *names.Suggester) func(context.Context, netaudit.Snapshot) {
	name := nameUnnamed(suggester)
	return func(ctx context.Context, snap netaudit.Snapshot) {
		name(ctx, snap)
		digest.Tick(ctx)
	}
}

// presenceHooks are what rides on the presence check: the work done on each look, and where its alert goes.
type presenceHooks struct {
	OnLook func(context.Context, netaudit.Snapshot)
	Notify func(context.Context, string) error
}

// newPresence builds the presence check. It sweeps through the scan service, so the declared scope and the one-scan-at-a-time
// rule apply; the alert goes to every connector, in counts and fixed words, and no model is involved.
func (s *AppServices) newPresence(service *netaudit.Service, inventory *netaudit.Inventory, config *netaudit.PresenceConfig, hooks presenceHooks) *netaudit.Presence {
	return netaudit.NewPresence(netaudit.PresenceDeps{
		Settings: config.Settings,
		Sweep: func(ctx context.Context, targets []string) error {
			_, err := service.Scan(ctx, netaudit.Request{Targets: targets, Mode: netaudit.ModeDiscover, Unattended: true})
			return err
		},
		Inventory: inventory,
		OnLook:    hooks.OnLook,
		Notify:    hooks.Notify,
		OnError:   func(err error) { logging.Warn("network presence check", "error", err.Error()) },
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

// newDecoys builds the decoy services. A connection counts only from a device inside the declared scan networks and not from
// this computer itself, whose own audit and sweep must never look like an intruder. The alert goes to every connector.
func (s *AppServices) newDecoys(config *netaudit.DecoyConfig, store *persistence.DecoyTouchStore, alerts *netaudit.AlertLog, scope *netaudit.Scope) *decoy.Manager {
	return decoy.New(decoy.Deps{
		Specs:   decoy.Specs,
		Enabled: config.Enabled,
		InScope: func(a netip.Addr) bool { return scope.Policy().Check(a) == nil },
		IsOwn:   ownAddress,
		Store:   store,
		Notify: func(ctx context.Context, sources []string) error {
			return alerts.Send(ctx, models.NetworkAlertDecoy, netaudit.DecoyMessage(sources), s.sendToConnectors)
		},
		OnError: func(err error) { logging.Warn("network decoys", "error", err.Error()) },
	})
}

// ownAddress reports whether a is one of this computer's own addresses (read from its interfaces; nothing is sent).
func ownAddress(a netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, ia := range addrs {
		if ipnet, ok := ia.(*net.IPNet); ok {
			if own, ok := netip.AddrFromSlice(ipnet.IP); ok && own.Unmap() == a {
				return true
			}
		}
	}
	return false
}
