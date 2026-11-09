package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"llm-proxy/internal/core/agentwatch"
	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/lockdown"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/knownbad"
	"llm-proxy/internal/core/netaudit/names"
	"llm-proxy/internal/core/netaudit/nmap"
	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/internal/core/netaudit/portprobe"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/core/netaudit/scantool"
	"llm-proxy/internal/core/netaudit/upnp"
	"llm-proxy/internal/core/netaudit/webprobe"
	"llm-proxy/internal/core/netaudit/wifilook"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/core/vulndata/ubuntuoval"
	"llm-proxy/internal/core/vulndata/vulnfeed"
	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/persistence"
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
	// Ubuntu holds Canonical's security notices for this computer's release and runs the operator-started update.
	Ubuntu *ubuntuoval.Manager
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
	// Router asks the router whether it offers UPnP, in the nightly audit only; with the outside lookup switched on it also asks
	// an outside scan database what the internet can reach.
	Router *netaudit.RouterCheck
	// OutsideLookupConfig is that lookup's switch and service; Exposure is the check, which keeps what its last look did.
	OutsideLookupConfig *netaudit.OutsideLookupConfig
	Exposure            *netaudit.Exposure
	// Network is the Network stack as it runs in the service (today the router DNS check, which moved into it); it is started with the
	// app. DNSCheckSwitch is that module's on/off switch over the settings the stack shares with the core.
	Network        *stackHost
	DNSCheckSwitch dnsCheckSwitch
	// Nearby is discovery over mDNS and SSDP (Constitution I.9) and the service checks that probe open ports.
	Nearby       *nearbyServices
	NearbyConfig *netaudit.NearbyConfig
	NearbyLooks  *nearbyLooks
	// DecoyConfig is the decoy services' switch; Decoys runs the listeners and DecoyStore keeps who connected.
	DecoyConfig *netaudit.DecoyConfig
	Decoys      *decoy.Manager
	DecoyStore  *persistence.DecoyTouchStore
	// RouterLogConfig is the switch for taking the router's log; RouterLog is the listener, which only wakes the presence check.
	RouterLogConfig *netaudit.RouterLogConfig
	RouterLog       *routerlog.Manager
	// Blocks is the count of the attempts the router refused; it writes them to the history every few minutes and at shutdown.
	Blocks *netaudit.Blocks
	// RouterWatch tells of a burst of failed router sign-ins or Wi-Fi disconnects; it runs from the one place the loops start.
	RouterWatch *netaudit.RouterWatch
	// LoginWatch counts the failed logins to this computer from the system's own record and tells of a burst; LoginWatchConfig is its switch.
	LoginWatch       *netaudit.LoginWatch
	LoginWatchConfig *netaudit.LoginWatchConfig
	// Wifi looks at the access points NetworkManager knows (a guarded nmcli) for a copy of the operator's network name; WifiConfig is its
	// switch and the operator's names; WifiReady says whether it can run.
	Wifi       *netaudit.WifiWatch
	WifiConfig *netaudit.WifiConfig
	WifiReady  func() (bool, string)
	// Alerts is the log of what the Network module alerted about, kept whether or not a connector exists.
	Alerts *netaudit.AlertLog
	// FollowUp scans a device that just arrived, once, while FollowUpConfig (off by default) allows it.
	FollowUp       *netaudit.FollowUp
	FollowUpConfig *netaudit.FollowUpConfig
	// SignIns sends the admin sign-in's lockouts and sign-ins from new addresses, from its own loop.
	SignIns *netaudit.SignInAlerts
	// AgentWatch is the agent security record: refused tool calls, approvals, new destinations of stable automations.
	AgentWatch *agentwatch.Watch
	// Monitors watches the module's own parts and alerts when one that is switched on stops working (monitor_stalled).
	Monitors *netaudit.MonitorWatch
	// CheckIn sends the daily "still running" note while CheckInConfig has it switched on.
	CheckIn       *netaudit.CheckIn
	CheckInConfig *netaudit.CheckInConfig
	// Stacks describes every switchable capability as a module of a stack; its switches are adapters over the settings above.
	Stacks *stack.Registry
	// KnownBadConfig is the switch for comparing names with abuse.ch's malware list; KnownBad holds the list; KnownBadList is the handlers' view of it.
	KnownBadConfig *netaudit.KnownBadConfig
	KnownBad       *knownbad.Manager
	KnownBadList   *knownBadList
	// WebChecksConfig is the nightly switch of the web checks; WebChecker asks the web pages their fixed questions.
	WebChecksConfig *netaudit.WebChecksConfig
	WebChecker      *webprobe.Prober
	// UDPChecksConfig is the nightly switch of the two UDP questions (Constitution item 12); UDPChecks asks them through the guarded dial.
	UDPChecksConfig *netaudit.UDPChecksConfig
	UDPChecks       *netaudit.UDPChecks
	// RunHealth records whether the optional steps of a nightly run worked; the monitor watch reads it.
	RunHealth *netaudit.RunHealth
	// PhoneHome is the watch of the names devices ask the router for; its switch is PhoneHomeConfig.
	PhoneHome *phonehome.Watch
	// PhoneHomeAlerts tells the operator of a new place or a steady beat; it runs from the one place the loops start.
	PhoneHomeAlerts *netaudit.PhoneHomeAlerts
	PhoneHomeConfig *netaudit.PhoneHomeConfig
	// AttackPathsConfig is the switch for the attack paths, which are on unless switched off.
	AttackPathsConfig *netaudit.AttackPathsConfig
	// Discovery6 is the switch for active IPv6 discovery, which the inventory runs on each look while it is on.
	Discovery6 *netaudit.Discovery6Config
	// Ports keeps the ports each scanned device had open at its last scan, for the audit's counts and the Devices card.
	Ports *netaudit.PortLog
	// Host compares this computer's open ports and the files that survive a reboot with its first look, in the nightly audit.
	Host *netaudit.HostWatcher
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

// NetauditNearby is where the run reporter reads what devices said about themselves; nil until the services are wired.
func (s AppServices) NetauditNearby() netaudit.NearbyFinder {
	if s.netaudit == nil || s.netaudit.Nearby == nil {
		return nil
	}
	return s.netaudit.Nearby.Findings
}

// NetauditClasses is the device-kind check the run reporter runs over a scan; nil until the services are wired.
func (s AppServices) NetauditClasses() netaudit.ClassFinder {
	if s.netaudit == nil || s.netaudit.Nearby == nil {
		return netaudit.ClassChecks{}
	}
	return netaudit.ClassChecks{State: s.netaudit.Nearby.Collector.State, Vendors: cardMakerOf}
}

// NetauditServices is the service checks the run reporter runs over a scan's open ports; nil until the services are wired.
func (s AppServices) NetauditServices() netaudit.ServiceFinder {
	if s.netaudit == nil || s.netaudit.Nearby == nil {
		return nil
	}
	return s.netaudit.Nearby.Services
}

// NetauditHistory is where the run reporter notes each audit's finding counts; nil until the services are wired.
func (s AppServices) NetauditHistory() netaudit.AuditHistory {
	if s.netaudit == nil || s.netaudit.History == nil {
		return nil
	}
	return s.netaudit.History
}

// NetauditPorts is where the run reporter keeps each scanned device's open ports; nil until the services are wired.
func (s AppServices) NetauditPorts() netaudit.PortRecorder {
	if s.netaudit == nil || s.netaudit.Ports == nil {
		return nil
	}
	return s.netaudit.Ports
}

// NetauditHost is where the run reporter compares this computer's open ports and startup files; nil until the services are wired.
func (s AppServices) NetauditHost() netaudit.HostLooker {
	if s.netaudit == nil || s.netaudit.Host == nil {
		return nil
	}
	return s.netaudit.Host
}

// hostIntegrityBudget is how long the nightly comparison of program files with their packages may take.
const hostIntegrityBudget = 3 * time.Minute

// newHostSource reads this computer's sockets from /proc and the closed list of watched files. startedAt identifies this run
// of the program, so replacing its file and restarting is not reported as tampering.
// hostPackages reads the installed packages and the Ubuntu release from the two files every account can read.
var hostPackages = hostwatch.Packages{StatusFile: "/var/lib/dpkg/status", OSRelease: "/etc/os-release"}

func newHostSource(startedAt time.Time) hostwatch.Source {
	home, _ := os.UserHomeDir() // an unknown home only leaves the account's own key file out
	exe, _ := os.Executable()
	return hostwatch.Source{
		Proc:    hostwatch.Proc{Root: "/proc"},
		Watch:   hostwatch.Files{Specs: hostwatch.DefaultFileSpecs(home, exe), ProgramTag: strconv.FormatInt(startedAt.Unix(), 10)},
		Harden:  hostwatch.Hardener{SSHDir: "/etc/ssh", SysDir: "/proc/sys", AptDir: "/etc/apt/apt.conf.d", UfwConf: "/etc/ufw/ufw.conf", KeyGlobs: keyGlobs(home), DiskPaths: diskPaths(home)},
		Patcher: hostwatch.Patcher{RebootFile: "/var/run/reboot-required", UpdatesFile: "/var/lib/update-notifier/updates-available", AutoUpgradeLog: "/var/log/unattended-upgrades/unattended-upgrades.log"},
		Docker:  hostwatch.Docker{NetDir: "/sys/class/net"},
		// A pass over the packaged program files; a longer one is cut short and reported as not finished.
		Pkgs:           hostPackages,
		Packages:       hostwatch.Integrity{InfoDir: "/var/lib/dpkg/info", Root: "/"},
		PackagesBudget: hostIntegrityBudget,
		Privileges:     privilegeReader(runtime.GOOS),
	}
}

// privilegeReader reads the privilege marks on Linux only. macOS has an /etc/passwd that leaves out the real accounts (they live in
// its directory service) and no /proc/modules, so reading it there would look like a check that found nothing; with no paths
// the check says "not checked" instead.
func privilegeReader(goos string) hostwatch.Privileger {
	if goos != "linux" {
		return hostwatch.Privileger{}
	}
	return hostwatch.Privileger{
		Passwd: "/etc/passwd", Group: "/etc/group", Modules: "/proc/modules", Preload: "/etc/ld.so.preload",
		SetIDRoots: []string{"/usr/local", "/opt", "/srv"},
	}
}

// keyGlobs lists the private key files the host watch checks the access of: the host keys, every account's id_* files and the
// service account's own.
func keyGlobs(home string) []string {
	globs := []string{"/etc/ssh/ssh_host_*_key", "/root/.ssh/id_*", "/home/*/.ssh/id_*"}
	if home != "" {
		globs = append(globs, filepath.Join(home, ".ssh", "id_*"))
	}
	return globs
}

// diskPaths are the folders whose volume's free space is checked: the system disk and the one holding the service account's home
// (where the program's data and, by default, the model files live).
func diskPaths(home string) []string {
	if home == "" {
		return []string{"/"}
	}
	return []string{"/", home}
}

// NetauditDecoys is where the run reporter reads the decoy touches; nil until the services are wired.
func (s AppServices) NetauditDecoys() netaudit.DecoyReader {
	if s.netaudit == nil || s.netaudit.DecoyStore == nil {
		return nil
	}
	return s.netaudit.DecoyStore
}

// NetauditAttackPaths is whether the report adds the attack paths; nil until the services are wired.
func (s AppServices) NetauditAttackPaths() func() bool {
	if s.netaudit == nil || s.netaudit.AttackPathsConfig == nil {
		return nil
	}
	return s.netaudit.AttackPathsConfig.Enabled
}

// NetauditWebChecks is the nightly web checks for the run reporter: whether they are switched on and what runs them, and the record
// of whether they worked; all nil until the services are wired.
func (s AppServices) NetauditWebChecks() (func() bool, netaudit.WebChecker, *netaudit.RunHealth) {
	if s.netaudit == nil || s.netaudit.WebChecksConfig == nil || s.netaudit.WebChecker == nil {
		return nil, nil, nil
	}
	return s.netaudit.WebChecksConfig.Enabled, s.netaudit.WebChecker, s.netaudit.RunHealth
}

// NetauditUDPChecks is the nightly UDP questions for the run reporter: whether they are switched on and what asks them; nil until the
// services are wired.
func (s AppServices) NetauditUDPChecks() (func() bool, netaudit.UDPFinder) {
	if s.netaudit == nil || s.netaudit.UDPChecksConfig == nil || s.netaudit.UDPChecks == nil {
		return nil, nil
	}
	return s.netaudit.UDPChecksConfig.Enabled, s.netaudit.UDPChecks
}

// NetauditPhoneHome is what the run reporter reads of the phone-home watch; nil until the services are wired.
func (s AppServices) NetauditPhoneHome() netaudit.PhoneHomeSource {
	if s.netaudit == nil || s.netaudit.PhoneHome == nil || s.netaudit.PhoneHomeConfig == nil {
		return nil
	}
	var bad func(string) (string, bool)
	if s.netaudit.KnownBadConfig != nil && s.netaudit.KnownBad != nil {
		bad = knownBad{Config: s.netaudit.KnownBadConfig, Manager: s.netaudit.KnownBad}.matcher()
	}
	return phoneHomeSignals{enabled: s.netaudit.PhoneHomeConfig.Enabled, watch: s.netaudit.PhoneHome, bad: bad}
}

// NetauditLoginWatch is what the run reporter reads of the failed logins to this computer; nil until the services are wired.
func (s AppServices) NetauditLoginWatch() netaudit.LoginSource {
	if s.netaudit == nil || s.netaudit.LoginWatch == nil {
		return nil
	}
	return s.netaudit.LoginWatch
}

// NetauditStackFindings is what the run reporter pulls the stacks' findings from; nil until the Network stack is hosted.
func (s AppServices) NetauditStackFindings() netaudit.StackFindingSource {
	if s.netaudit == nil || s.netaudit.Network == nil {
		return nil
	}
	return s.netaudit.Network
}

// NetauditWifi is what the run reporter reads of the Wi-Fi look-alike watch; nil until the services are wired.
func (s AppServices) NetauditWifi() netaudit.WifiSource {
	if s.netaudit == nil || s.netaudit.Wifi == nil {
		return nil
	}
	return s.netaudit.Wifi
}

// NetauditRouterLog is what the run reporter reads of the router's log; nil until the services are wired.
func (s AppServices) NetauditRouterLog() netaudit.RouterLogSource {
	if s.netaudit == nil || s.netaudit.RouterWatch == nil {
		return nil
	}
	return routerLogSource{watch: s.netaudit.RouterWatch, gateway: cachedGateway(neighborSource{reader: neighbors.NewReader()}, gatewayTTL, time.Now)}
}

// routerLogSource gives the report the watch's snapshot and the router's address.
type routerLogSource struct {
	watch   *netaudit.RouterWatch
	gateway func() netip.Addr
}

func (r routerLogSource) Snapshot(now time.Time) netaudit.RouterSnapshot { return r.watch.Snapshot(now) }

func (r routerLogSource) Router() string {
	if gw := r.gateway(); gw.IsValid() {
		return gw.String()
	}
	return ""
}

// StartNetauditPresence runs the Network module's background work until ctx ends: the presence check, the router log listener
// and its counter of refused attempts, the DNS comparison, the decoys, Nearby and the watch over them all. It is the one place
// that starts them (Constitution II.2, II.14); work that is switched off in the settings does nothing.
func (s AppServices) StartNetauditPresence(ctx context.Context) {
	for _, run := range s.netauditLoops() {
		go run(ctx)
	}
}

// netauditLoops lists the Network module's background loops that are wired; each runs until its context ends.
func (s AppServices) netauditLoops() []func(context.Context) {
	n := s.netaudit
	if n == nil {
		return nil
	}
	var loops []func(context.Context)
	add := func(wired bool, run func(context.Context)) {
		if wired {
			loops = append(loops, run)
		}
	}
	add(n.Presence != nil, func(ctx context.Context) { n.Presence.Run(ctx) })
	add(n.RouterLog != nil, func(ctx context.Context) { n.RouterLog.Run(ctx) })
	add(n.PhoneHomeAlerts != nil, func(ctx context.Context) { n.PhoneHomeAlerts.Run(ctx) })
	add(n.Blocks != nil, func(ctx context.Context) { n.Blocks.Run(ctx) })
	add(n.RouterWatch != nil, func(ctx context.Context) { n.RouterWatch.Run(ctx) })
	add(n.LoginWatch != nil, func(ctx context.Context) { n.LoginWatch.Run(ctx) })
	add(n.Wifi != nil, func(ctx context.Context) { n.Wifi.Run(ctx) })
	add(n.PhoneHome != nil, func(ctx context.Context) {
		n.PhoneHome.Run(ctx, func(err error) { logging.Warn("network phone-home", "error", err.Error()) })
	})
	add(n.Network != nil, func(ctx context.Context) { n.Network.Run(ctx) })
	add(n.Decoys != nil, func(ctx context.Context) { n.Decoys.Run(ctx) })
	add(n.Nearby != nil, func(ctx context.Context) { n.Nearby.Collector.Run(ctx) })
	add(n.Monitors != nil, func(ctx context.Context) { n.Monitors.Run(ctx) })
	add(n.SignIns != nil, func(ctx context.Context) { n.SignIns.Run(ctx) })
	add(n.FollowUp != nil, func(ctx context.Context) { n.FollowUp.Run(ctx) })
	add(n.AgentWatch != nil, func(ctx context.Context) { n.AgentWatch.Run(ctx) })
	add(n.CheckIn != nil, func(ctx context.Context) { n.CheckIn.Run(ctx) })
	return loops
}

// workspaceDirs lists every workspace and its folder, for the agent watch's nightly secrets pass.
func (s AppServices) workspaceDirs() ([]agentwatch.WorkspaceDir, error) {
	if s.Persistence() == nil {
		return nil, nil
	}
	list, err := s.Persistence().ListWorkspaces()
	if err != nil {
		return nil, err
	}
	resolver := storage.NewPathResolver(s.AppCtx.RootDir(), s.AppCtx.WorkspacesDir(), s.AppCtx.MetadataDir())
	out := make([]agentwatch.WorkspaceDir, 0, len(list))
	for _, w := range list {
		out = append(out, agentwatch.WorkspaceDir{ID: w.ID, Dir: resolver.WorkspaceDir(w.ID)})
	}
	return out, nil
}

// AttachAgentWatch gives the agent security record every event the bus publishes. It is called once, when the dispatcher (which
// owns the bus) exists; without the Network services it does nothing.
func (s AppServices) AttachAgentWatch(bus interface {
	SetTap(func(string, assistantPkg.AgentEvent))
}) {
	if s.netaudit == nil || s.netaudit.AgentWatch == nil || bus == nil {
		return
	}
	bus.SetTap(s.netaudit.AgentWatch.Observe)
}

// NetauditMonitors is what the nightly report reads about the module's own parts that stopped; nil until the services are wired.
func (s AppServices) NetauditMonitors() netaudit.MonitorStalls {
	if s.netaudit == nil || s.netaudit.Monitors == nil {
		return nil
	}
	return s.netaudit.Monitors
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

// LockedDownCode is the pre-flight reason for any automation while lock down is on.
const LockedDownCode = "locked_down"

// RunPreflight is the check every automation passes before the model is touched: lock down first, then the Network module's
// own checks for an audit.
func (s AppServices) RunPreflight() automation.Preflight {
	network := s.NetauditPreflight()
	return func(ctx context.Context, entry *automation.AutomationEntry) error {
		if s.AppCtx != nil && s.AppCtx.LockedDown() {
			return &automation.PreflightError{Code: LockedDownCode, Message: lockdown.Message}
		}
		if network != nil {
			return network(ctx, entry)
		}
		return nil
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
	if !known || s.isLocalModel(cfg) {
		return nil
	}
	return &automation.PreflightError{
		Code:    ModelNotLocalCode,
		Message: fmt.Sprintf("The model %q runs in the cloud, and a network audit may only use a model on this machine. Choose a local model for the automation.", name),
	}
}

// modelIsLocal reports whether a model runs on this machine. A model that cannot be found is not local: a scheduled audit
// leaves that to fail in its run, but the timeline summary sends its data at once, so it only goes where it is known to stay.
func (s AppServices) modelIsLocal(name string) bool {
	cfg, known := s.ModelConfig(name)
	return known && s.isLocalModel(cfg)
}

// isLocalModel is the one answer to "does this model run on this machine" for the Network module: the class the runtime resolved
// from the model's effective endpoint, or, for a model it has not resolved yet, the classifier's answer from the stored config.
func (s AppServices) isLocalModel(cfg models.ModelConfig) bool {
	if cfg.WorkloadClass != "" {
		return cfg.WorkloadClass == models.WorkloadLocal
	}
	return s.workloadClassifier.Classify(cfg) == models.WorkloadLocal
}

// wireNetaudit builds the network-audit services and registers the network_scan tool. It is the one place they are
// put together: the policy comes from settings, the engines from the sandbox provider, and the tool is hidden whenever
// the readiness check says a scan cannot run. Each stage builds one concern and hands back what the next one needs.
func (s *AppServices) wireNetaudit(dataMgr *storage.DataManager) {
	store := dataMgr.Netaudit()
	scope := netaudit.NewScope(store, net.DefaultResolver, nil)
	// The vulnerability data, device names and router check reach the network through the guarded client, so the global
	// network guardrails and the blocked-domain list apply to them like to any agent fetch.
	guard := func() models.NetworkGuardrailsConfig { return dataMgr.Settings().Get().Guardrails.Network }
	netTools := tools.NewNetworkTools(func(context.Context) models.NetworkGuardrailsConfig { return guard() }, s.logger)

	scan := s.newScanning(store, scope, netTools)
	known := s.newTracking(store, scope, netTools)
	phone := s.newPhoneHome(store, scope)
	bad := s.newKnownBad(store, netTools)
	webChecker := webprobe.New(func(a netip.Addr) error { return scope.Policy().Check(a) })
	alerts := s.newAlerting(alertingDeps{Store: store, Scope: scope, Service: scan.Service, Tracking: known, PhoneHome: phone})

	outside := newOutsideLookup(store, netTools, upnpClient(netTools))
	network, err := s.newNetworkHost(store, netTools, scope)
	if err != nil {
		s.logger.Error("the Network stack could not be hosted", "error", err.Error())
	}
	near := s.newNearby(store, scope, netTools, guard, known.Inventory)

	// Canonical's notices for this release are fetched only when the operator asks, and judge the installed packages once held.
	ubuntu := ubuntuoval.New(persistence.NewUbuntuIndexStore(s.AppCtx.RootDir()), netTools.Open, hostPackages.Release)
	s.AppCtx.AfterFactoryReset("The downloaded Ubuntu security data", ubuntu.Clear)
	known.Host.WithFlaws(netaudit.PackageFlaws{Index: ubuntu.Index, Exploited: func() map[string]string { return scan.Vulns.Index().Exploited }, Release: hostPackages.Release})

	s.netaudit = &netauditServices{
		Scope:     scope,
		Readiness: scan.Readiness,
		Vulns:     scan.Vulns,
		Ubuntu:    ubuntu,

		Inventory:         known.Inventory,
		Identities:        known.Identities,
		History:           known.History,
		Names:             known.Names,
		NamesConfig:       known.NamesConfig,
		Summary:           known.Summary,
		Discovery6:        known.Discovery6,
		AttackPathsConfig: netaudit.NewAttackPathsConfig(store),
		KnownBadConfig:    bad.Config,
		KnownBad:          bad.Manager,
		KnownBadList:      bad.List,
		WebChecksConfig:   netaudit.NewWebChecksConfig(store),
		WebChecker:        webChecker,
		UDPChecksConfig:   netaudit.NewUDPChecksConfig(store),
		UDPChecks:         &netaudit.UDPChecks{Asker: stackUDPAsker{host: network}},
		RunHealth:         netaudit.NewRunHealth(),
		PhoneHome:         phone.Watch,
		PhoneHomeConfig:   phone.Config,
		Ports:             known.Ports,
		Host:              known.Host,
		Router:            newRouterCheck(netTools, known.Inventory, scope).WithExposure(outside.Exposure),

		OutsideLookupConfig: outside.Config,
		Exposure:            outside.Exposure,
		Network:             network,
		DNSCheckSwitch:      dnsCheckSwitch{store},
		Nearby:              &near,
		NearbyConfig:        near.Config,
		NearbyLooks:         near.Looks,

		Alerts:         alerts.Log,
		Presence:       alerts.Presence,
		PresenceConfig: alerts.PresenceConfig,
		Decoys:         alerts.Decoys,
		DecoyConfig:    alerts.DecoyConfig,
		DecoyStore:     alerts.DecoyStore,

		RouterLogConfig:  alerts.RouterLogConfig,
		RouterLog:        alerts.RouterLog,
		Blocks:           alerts.Blocks,
		RouterWatch:      alerts.RouterWatch,
		LoginWatch:       alerts.LoginWatch,
		LoginWatchConfig: alerts.LoginWatchConfig,
		FollowUp:         alerts.FollowUp,
		FollowUpConfig:   alerts.FollowUpConfig,

		Acks:         netaudit.NewAcks(s.Persistence(), s.Persistence()),
		Fixes:        netaudit.NewFixes(s.Persistence(), s.Persistence()),
		Expectations: netaudit.NewExpectations(s.Persistence(), scope),
		Deep:         netaudit.NewDeepChecks(scan.Service, s.Persistence(), s.Persistence()).WithWebChecks(webChecker),
	}
	s.netaudit.WifiConfig = netaudit.NewWifiConfig(store)
	s.netaudit.Wifi = netaudit.NewWifiWatch(netaudit.WifiWatchDeps{
		List:    wifilook.New(scan.Runner).List,
		Config:  s.netaudit.WifiConfig,
		Notify:  s.alertNotifier(alerts.Log, models.NetworkAlertWifi),
		OnError: func(err error) { logging.Warn("network wifi look-alikes", "error", err.Error()) },
	})
	s.netaudit.WifiReady = s.wifiReady(store, s.netaudit.WifiConfig)
	s.netaudit.PhoneHomeAlerts = netaudit.NewPhoneHomeAlerts(netaudit.PhoneHomeAlertDeps{
		Watch: phone.Watch, Enabled: phone.Config.Enabled, Inventory: known.Inventory.View, KnownBad: bad.matcher(),
		Notify:  s.alertNotifier(alerts.Log, models.NetworkAlertPhoneHome),
		OnError: func(err error) { logging.Warn("network phone-home alerts", "error", err.Error()) },
	})
	s.netaudit.Monitors = newMonitorWatch(s.netaudit, s.alertNotifier(alerts.Log, models.NetworkAlertMonitor))
	s.netaudit.SignIns = netaudit.NewSignInAlerts(s.alertNotifier(alerts.Log, models.NetworkAlertSignIn), time.Now)
	s.netaudit.CheckInConfig = netaudit.NewCheckInConfig(store)
	s.netaudit.CheckIn = netaudit.NewCheckIn(netaudit.CheckInDeps{
		Enabled: s.netaudit.CheckInConfig.Enabled, History: known.History, Stalled: s.netaudit.Monitors.Stalled,
		// Straight to the connectors, not through the alert log: its only reader is the operator's phone, and a daily entry would
		// be noise in the bell and the Timeline.
		Notify:  s.sendToConnectors,
		OnError: func(err error) { logging.Warn("network daily check-in", "error", err.Error()) },
	})
	agents := dataMgr.Agents()
	s.netaudit.AgentWatch = agentwatch.New(agentwatch.Deps{
		Store:         withReset(s.AppCtx, "The agent security record", persistence.NewAgentWatchStore(s.AppCtx.RootDir())),
		Notify:        s.alertNotifier(alerts.Log, models.NetworkAlertAgent),
		OnError:       func(err error) { logging.Warn("agent watch", "error", err.Error()) },
		Workspaces:    s.workspaceDirs,
		ActivityOn:    agentActivitySwitch(agents).Enabled,
		SecretsPassOn: secretsPassSwitch(agents).Enabled,
	})

	// The registry is a static description checked by tests; one that is inconsistent is a bug to stop at start, not to serve.
	registry, err := newStackRegistry(stackDeps{Network: s.netaudit, Agents: agents, Guard: guard})
	if err != nil {
		panic(err)
	}
	s.netaudit.Stacks = registry

	if registry := localRegistry(s.toolProvider); registry != nil {
		registry.Use(scantool.Module(scan.Service, s.Persistence(), func(context.Context) bool { return len(scan.Readiness.Blocking()) == 0 }))
	}
}

// scanning is what runs and vets a scan: the service, the readiness check over it, and the vulnerability data.
type scanning struct {
	Service   *netaudit.Service
	Readiness *netaudit.Readiness
	Vulns     *vulnfeed.Manager
	// Runner starts guarded programs (nmap) under the sandbox.
	Runner isolation.Runner
}

func (s *AppServices) newScanning(store netaudit.PolicyStore, scope *netaudit.Scope, netTools *tools.NetworkTools) scanning {
	runner := isolation.NewRunner(s.AppCtx.sandboxProv, func() bool { return store.Get().AllowUnconfined })
	nmapEngine := nmap.New(runner)
	probe := portprobe.New(func(a netip.Addr) error { return scope.Policy().Check(a) })
	service := netaudit.NewService(
		netaudit.PolicyAuthorizer{Policy: scope.Policy, Resolver: net.DefaultResolver}, nmapEngine, probe).WithDeviceTimeout(scope.DeviceTimeout)

	// The vulnerability data is downloaded only when the operator asks. It lives beside the settings, outside what a
	// factory reset replaces, so a reset clears it too.
	vulns := vulnfeed.New(persistence.NewVulnIndexStore(s.AppCtx.RootDir()), netTools.Open, vulnfeed.DefaultSources())
	s.AppCtx.AfterFactoryReset("The downloaded vulnerability data", vulns.Clear)

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
	return scanning{Service: service, Readiness: readiness, Vulns: vulns, Runner: runner}
}

// tracking is what the module remembers about the network: who is on it, what they are called, and how it changes.
type tracking struct {
	Inventory   *netaudit.Inventory
	Identities  *netaudit.Identities
	History     *netaudit.History
	Names       *names.Suggester
	NamesConfig *netaudit.NamesConfig
	Summary     *netaudit.SummaryConfig
	Discovery6  *netaudit.Discovery6Config
	Ports       *netaudit.PortLog
	Host        *netaudit.HostWatcher
}

func (s *AppServices) newTracking(store netaudit.PolicyStore, scope *netaudit.Scope, netTools *tools.NetworkTools) tracking {
	root := s.AppCtx.RootDir()
	historyStore := withReset(s.AppCtx, "The daily counts behind the Network page's trends", persistence.NewNetworkHistoryStore(root))
	history := netaudit.NewHistory(historyStore)
	portStore := withReset(s.AppCtx, "The ports each device had open at its last scan", persistence.NewDevicePortStore(root))
	discovery6 := netaudit.NewDiscovery6Config(store)
	namesConfig := netaudit.NewNamesConfig(store)

	nameStore := withReset(s.AppCtx, "The names devices gave when asked", persistence.NewNameSuggestionStore(root))
	ask := scopedAsker(names.Prober{Dial: netTools.DialContext()}.Ask, scope)
	suggester := names.NewSuggester(nameStore, ask).WithEnabled(namesConfig.Enabled)

	inventoryStore := withReset(s.AppCtx, "The list of devices seen on the network", persistence.NewDeviceInventoryStore(root))
	inScope := func(a netip.Addr) bool { return scope.Policy().Check(a) == nil }
	inventory := netaudit.NewInventory(inventoryStore, neighborSource{reader: neighbors.NewReader()}, inScope).
		WithDiscovery6(echoDiscoverer{}, discovery6.Enabled).
		WithObserver(recordLook(history))

	identityStore := withReset(s.AppCtx, "The remembered SSH host keys and certificates", persistence.NewDeviceIdentityStore(root))
	hostStore := withReset(s.AppCtx, "The first look of the host watch (open ports and watched files)", persistence.NewHostWatchStore(root))
	return tracking{
		Host:        netaudit.NewHostWatcher(newHostSource(time.Now()), hostStore),
		Inventory:   inventory,
		Identities:  netaudit.NewIdentities(identityStore),
		History:     history,
		Names:       suggester,
		NamesConfig: namesConfig,
		Summary:     netaudit.NewSummaryConfig(store),
		Discovery6:  discovery6,
		Ports:       netaudit.NewPortLog(portStore).WithHistory(history),
	}
}

// withReset registers a store's Delete to run on a factory reset and returns the store, so a store and the reset of its
// file are one expression.
func withReset[S interface{ Delete() error }](app *AppContext, what string, store S) S {
	app.AfterFactoryReset(what, store.Delete)
	return store
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

// netauditPresence joins the presence check's settings with the check's own status for the handlers.
type netauditPresence struct {
	*netaudit.PresenceConfig
	check *netaudit.Presence
}

func (p netauditPresence) Status() netaudit.PresenceStatus { return p.check.Status() }

// upnpClient talks to the router. Both the datagram and the page fetch go through the guarded network client.
func upnpClient(netTools *tools.NetworkTools) upnp.Client {
	return upnp.Client{Dial: netTools.DialContext(), HTTP: netTools.HTTPClient()}
}

// newRouterCheck builds the UPnP check of the router. Both the datagram and the page fetch go through the guarded network
// client, so the global network guardrails apply, and the router must be inside the declared scan scope.
func newRouterCheck(netTools *tools.NetworkTools, inventory *netaudit.Inventory, scope *netaudit.Scope) *netaudit.RouterCheck {
	client := upnpClient(netTools)
	gateway := func() (netip.Addr, error) {
		inv, err := inventory.View()
		if err != nil || inv.Gateway == "" {
			return netip.Addr{}, err
		}
		return netip.ParseAddr(inv.Gateway)
	}
	return netaudit.NewRouterCheck(client, gateway, func(a netip.Addr) error { return scope.Policy().Check(a) })
}

// NetauditOwnModel says which address and ports are this program's own model server, so the audit names the setting that closes
// it instead of the generic AI-server guide. The ports are those of the local models in the catalogue, read at report time.
func (s AppServices) NetauditOwnModel() netaudit.OwnModelServer {
	if s.netaudit == nil || s.netaudit.Scope == nil || s.AppCtx == nil {
		return netaudit.OwnModelServer{}
	}
	return netaudit.OwnModelServer{
		IsOwnAddr: cachedOwnAddress(s.netaudit.Scope, ownAddressTTL, time.Now),
		Ports:     func() []int { return localModelPorts(s.AppCtx.GetRegistry()) },
	}
}

// localModelPorts are the ports the catalogue's local models are started on.
func localModelPorts(reg models.RegistryData) []int {
	var out []int
	for _, m := range reg.Catalogue {
		if m.ProviderID == models.ProviderLocal && m.Port > 0 {
			out = append(out, m.Port)
		}
	}
	return out
}

// wifiReady says whether the Wi-Fi look can run, and when it cannot, why in plain words: nmcli must be installed, programs must be
// confined on this computer (or the operator has allowed running them unconfined), and the operator must have named a network.
func (s *AppServices) wifiReady(store netaudit.PolicyStore, cfg *netaudit.WifiConfig) func() (bool, string) {
	return func() (bool, string) {
		if err := wifilook.Installed(); err != nil {
			return false, err.Error()
		}
		if s.AppCtx.sandboxProv.Effective().Filesystem.Mechanism == isolation.EnforcementNone && !store.Get().AllowUnconfined {
			return false, "programs are not confined on this computer, so this one is not run (see the sandbox settings)"
		}
		if len(cfg.Networks()) == 0 {
			return false, "add your Wi-Fi network's name on the Settings tab"
		}
		return true, ""
	}
}
