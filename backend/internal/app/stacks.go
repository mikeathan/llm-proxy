package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"slices"
	"time"

	"llm-proxy-stacks/agents"
	"llm-proxy-stacks/network"
	"llm-proxy-stacks/network/oui"
	"llm-proxy-stacks/radio"
	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/agentwatch"
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/loginlog"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// cardMakerOf names the maker of a network card. The registry is the Network stack's data (stacks/network/oui), so the core never imports it:
// the composition root, the one place that may, hands this function to the code that shows or judges a device by its maker.
var cardMakerOf netaudit.VendorLookup = oui.Vendor

// networkUDPModule is the id of the nightly UDP checks, the sensor the Network stack answers for.
const networkUDPModule = network.UDPModuleID

// networkWebModule is the id of the nightly web checks, the second sensor the Network stack answers for.
const networkWebModule = network.WebModuleID

// networkRouterModule is the id of the router check's sensor, the third the Network stack answers for. It has no switch and no registry entry.
const networkRouterModule = network.RouterModuleID

// The registry of stacks and modules (ADR-0028) is described here, once, in the composition root. It owns no setting: each module
// is a Switch over the setting that already holds it, doing what that feature's own route did when it was switched.

// agentsPolicy is the Agents stack's stored switches: *storage's agents view.
type agentsPolicy interface {
	Get() models.AgentsConfig
	Update(func(*models.AgentsConfig) error) error
}

// radioPolicy is the Radio stack's stored switch: *storage's radio view.
type radioPolicy interface {
	Get() models.RadioConfig
	Update(func(*models.RadioConfig) error) error
}

// stackDeps are what the registry is built from.
type stackDeps struct {
	Network *netauditServices
	Agents  agentsPolicy
	Radio   radioPolicy
	// Guard reads the network Guardrails each time, so a change in Settings shows at once.
	Guard func() models.NetworkGuardrailsConfig
}

// Texts the screen shows for what blocks a module; fixed words.
const (
	blockedNoLAN      = "local network access is off in Guardrails"
	blockedNoInternet = "internet access is off in Guardrails"
	blockedNoNetwork  = "network access is off in Guardrails"
)

func newStackRegistry(d stackDeps) (*stack.Registry, error) {
	reg, err := stack.New(stackDefinitions(), described(slices.Concat(networkModules(d), agentModules(d), radioModules(d))))
	if err != nil {
		return nil, fmt.Errorf("failed to build the stack registry: %w", err)
	}
	return reg, nil
}

func stackDefinitions() []stack.Stack {
	return []stack.Stack{
		{
			ID: stack.Network, Title: "Network", Purpose: "Watch this network, and what it says to the world.",
			Layers: []stack.Layer{
				{ID: "discover", Title: "Who is on the network", Purpose: "Devices, and what they are called"},
				{ID: "listen", Title: "What the network says", Purpose: "What the router and the devices tell you by themselves"},
				{ID: "test", Title: "Look closer", Purpose: "Checks you switch on that look at devices on purpose"},
				{ID: "outside", Title: "What the internet sees", Purpose: "Your network from the outside"},
				{ID: "understand", Title: "Decide what to fix", Purpose: "The findings joined into what matters most"},
				{ID: "tell", Title: "Reach you", Purpose: "How the Network module gets in touch"},
			},
		},
		{
			ID: stack.Agents, Title: "Agents", Purpose: "Watch what the agents on this computer do.",
			Layers:  []stack.Layer{{ID: "watch", Title: "Watch", Purpose: "What the agents did"}},
			Actions: []stack.Action{{ID: "lockdown", Title: "Lock down", Purpose: "Stop the agents and sign out every other session."}},
		},
		{
			ID: stack.Radio, Title: "Radio", Purpose: "Listen to the radio around this computer, with the adapters you plug in.",
			Layers: []stack.Layer{{ID: "sensors", Title: "Sensors", Purpose: "The radios this computer has, and what each can do"}},
		},
	}
}

// networkModules are the switches of the Network module, each over the config type that already owned it.
func networkModules(d stackDeps) []stack.Module {
	n, guard := d.Network, d.Guard
	lan := func() (bool, string) { return networkReady(guard(), true, false) }
	internet := func() (bool, string) { return networkReady(guard(), false, true) }
	both := func() (bool, string) { return networkReady(guard(), true, true) }
	loginReady := func() (bool, string) { return loginRecordReady(loginlog.Reader{Path: loginlog.DefaultPath}) }
	return []stack.Module{
		{ID: "presence", Title: "Presence check", Purpose: "Notice new and missing devices.", Stack: stack.Network, Layer: "discover", Sends: stack.SendsLAN,
			Switch: presenceSwitch{n.PresenceConfig}, Monitors: []string{netaudit.MonitorPresence}},
		{ID: "ipv6_discovery", Title: "IPv6 discovery", Purpose: "Find devices that speak only IPv6.", Stack: stack.Network, Layer: "discover", Sends: stack.SendsLAN,
			Switch: n.Discovery6},
		{ID: "device_names", Title: "Device names", Purpose: "Ask devices what they are called.", Stack: stack.Network, Layer: "discover", Sends: stack.SendsLAN,
			Switch: n.NamesConfig},
		{ID: "nearby", Title: "Nearby", Purpose: "What devices announce about themselves.", Stack: stack.Network, Layer: "discover", Sends: stack.SendsLAN, StepUp: true,
			Switch: n.NearbyConfig, Ready: lan},
		{ID: "router_log", Title: "Router log", Purpose: "Refused attempts and new devices, from the router.", Stack: stack.Network, Layer: "listen", Sends: stack.Listens, StepUp: true,
			Switch: withPoke(n.RouterLogConfig, func() { n.RouterLog.Poke() }), Monitors: []string{netaudit.MonitorRouterLog, netaudit.MonitorRouterLogQuiet}},
		{ID: "decoys", Title: "Decoys", Purpose: "Fake services that catch probing.", Stack: stack.Network, Layer: "listen", Sends: stack.Listens, StepUp: true,
			Switch: withPoke(n.DecoyConfig, func() { n.Decoys.Poke() }), Monitors: []string{netaudit.MonitorDecoys}},
		{ID: "phone_home", Title: "Phone-home watch", Purpose: "Where each device calls out to.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsNothing,
			Needs: []string{"router_log"}, Switch: n.PhoneHomeConfig, Monitors: []string{netaudit.MonitorPhoneHome}},
		{ID: "router_connections", Title: "Router connections", Purpose: "Where each device connects, read from the router once a night.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsLAN, StepUp: true,
			Switch: n.RouterConnConfig, Monitors: []string{netaudit.MonitorRouterConn}, Ready: func() (bool, string) { return routerConnReady(lan, n.RouterConn) }},
		{ID: "login_watch", Title: "Failed logins, this computer", Purpose: "Tells you when someone keeps failing to log in here.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsNothing,
			Switch: n.LoginWatchConfig, Monitors: []string{netaudit.MonitorLoginWatch}, Ready: loginReady},
		{ID: "wifi_lookalikes", Title: "Wi-Fi look-alikes", Purpose: "Tells you when a Wi-Fi network copies yours.", Stack: stack.Network, Layer: "discover", Sends: stack.SendsNothing,
			Switch: n.WifiConfig, Monitors: []string{netaudit.MonitorWifi}, Ready: n.WifiReady},
		{ID: "known_bad", Title: "Known-bad names", Purpose: "Warns when a device asks for a host that serves malware.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsNothing,
			Needs: []string{"phone_home"}, Switch: n.KnownBadConfig, Ready: n.KnownBadList.Ready},
		{ID: "scan_on_arrival", Title: "Scan on arrival", Purpose: "Scan a new device once.", Stack: stack.Network, Layer: "test", Sends: stack.SendsLAN, StepUp: true,
			Needs: []string{"presence"}, Switch: n.FollowUpConfig},
		{ID: networkWebModule, Title: "Web and certificate checks, nightly", Purpose: "Looks at the web page and certificate of every device the audit finds.", Stack: stack.Network, Layer: "test", Sends: stack.SendsLAN, StepUp: true,
			Ready: lan, Switch: n.WebChecksConfig, Monitors: []string{netaudit.MonitorWebChecks}},
		{ID: networkUDPModule, Title: "UDP checks, nightly", Purpose: "Asks each device whether its SNMP or UPnP service answers.", Stack: stack.Network, Layer: "test", Sends: stack.SendsLAN, StepUp: true,
			Ready: lan, Switch: n.UDPChecksConfig},
		{ID: "outside_lookup", Title: "Internet view", Purpose: "Ports the internet can reach.", Stack: stack.Network, Layer: "outside", Sends: stack.SendsInternet, StepUp: true,
			Switch: n.OutsideLookupConfig, Ready: internet},
		{ID: "dns_check", Title: "Router DNS check", Purpose: "Is the router answering honestly?", Stack: stack.Network, Layer: "outside", Sends: stack.SendsInternet, StepUp: true,
			Switch: n.DNSCheckSwitch, Monitors: []string{netaudit.MonitorDNS}, Ready: both},
		{ID: "attack_paths", Title: "Attack paths", Purpose: "The ways in that matter, and the one fix for each.", Stack: stack.Network, Layer: "understand", Sends: stack.SendsNothing,
			Switch: n.AttackPathsConfig},
		{ID: "weekly_summary", Title: "Weekly summary", Purpose: "Counts of the week to your phone.", Stack: stack.Network, Layer: "tell", Sends: stack.SendsMessage,
			Needs: []string{"presence"}, Switch: n.Summary},
		{ID: "daily_check_in", Title: "Daily check-in", Purpose: "A daily “still running” note.", Stack: stack.Network, Layer: "tell", Sends: stack.SendsMessage,
			Switch: n.CheckInConfig},
	}
}

// networkReady says whether the guarded network client would let the module through: the master switch, then the local network
// and the internet as the module needs them.
func networkReady(g models.NetworkGuardrailsConfig, needLAN, needInternet bool) (bool, string) {
	switch {
	case !g.Enabled:
		return false, blockedNoNetwork
	case needLAN && !g.AllowLanAccess:
		return false, blockedNoLAN
	case needInternet && !g.AllowInternetAccess:
		return false, blockedNoInternet
	}
	return true, ""
}

// agentModules are the two parts of the Agents stack. Both have always run, so each is stored as an "off" flag; switching either
// off silences a security record, so it needs the passphrase like erasing the decoys' record does.
func agentModules(d stackDeps) []stack.Module {
	return []stack.Module{
		{ID: "agent_activity", Title: "Agent activity", Purpose: "Refused tool calls, approvals and new destinations.", Stack: stack.Agents, Layer: "watch", Sends: stack.SendsMessage, StepUp: true,
			Switch: agentActivitySwitch(d.Agents)},
		{ID: "secrets_pass", Title: "Secrets pass", Purpose: "A nightly look through workspace files for credentials.", Stack: stack.Agents, Layer: "watch", Sends: stack.SendsMessage, StepUp: true,
			Switch: secretsPassSwitch(d.Agents)},
	}
}

// radioModules are the Radio stack's switches. The stack is off until the operator opts in, and it sends nothing, so its switch needs no passphrase.
func radioModules(d stackDeps) []stack.Module {
	return []stack.Module{
		{ID: radio.SensorsModuleID, Title: "Radio sensors", Purpose: "Find the radios on this computer and say what each can do.", Stack: stack.Radio, Layer: "sensors", Sends: stack.SendsNothing,
			Switch: radioSwitch{d.Radio}},
	}
}

// radioSwitch is the Radio policy's one flag read as a switch.
type radioSwitch struct{ store radioPolicy }

func (r radioSwitch) Enabled() bool { return r.store.Get().Enabled }

func (r radioSwitch) Set(enabled bool) error {
	return r.store.Update(func(c *models.RadioConfig) error {
		c.Enabled = enabled
		return nil
	})
}

// presenceSwitch turns the presence check on or off without touching the interval the operator chose.
type presenceSwitch struct{ *netaudit.PresenceConfig }

func (p presenceSwitch) Enabled() bool { return p.View().Enabled }

func (p presenceSwitch) Set(enabled bool) error { return p.SetEnabled(enabled) }

// dnsCheckSwitch is the DNS check's switch alone, over the settings document the Network stack shares with the core; its names, server
// and interval stay on its own card, which the stack serves.
type dnsCheckSwitch struct{ store netaudit.PolicyStore }

func (d dnsCheckSwitch) Enabled() bool { return d.store.Get().DNSCheckEnabled }

func (d dnsCheckSwitch) Set(enabled bool) error {
	if err := d.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the DNS check setting: %w", err)
	}
	return nil
}

// networkStack is the Network stack. Only this file imports a stack (Constitution IV.6).
func networkStack() stacksdk.Stack { return network.New() }

// newNetworkHost hosts the Network stack over the core's own services, one adapter per port its manifest requires: the settings document
// (the `netaudit` key, unchanged), the guarded dial and fetch, the default gateway the neighbour table gives, the service's log, and the
// data folder restricted to the files the manifest lists. The files it lists are removed by a factory reset, as the store they replace was.
func (s *AppServices) newNetworkHost(store netaudit.PolicyStore, netTools *tools.NetworkTools, scope *netaudit.Scope) (*stackHost, error) {
	st := networkStack()
	files := st.Manifest().Storage
	if _, err := claimedFiles(st.Manifest()); err != nil {
		return nil, err
	}
	dir := s.AppCtx.RootDir()
	gateway := cachedGateway(neighborSource{reader: neighbors.NewReader()}, gatewayTTL, time.Now)
	host := stacksdk.Host{
		Config:  stackport.Config[models.NetauditConfig]{Store: store},
		Log:     stackport.Log{Stack: st.Manifest().ID},
		DataDir: stackport.DataDir{Dir: dir, Files: files},
		Guard:   stackport.Guard{Dial: netTools.DialContext()},
		Fetch:   stackport.Fetch{Opener: netTools},
		Gateway: stackport.Gateway{Read: gateway},
		// A sensor's datagrams go through the guarded dial after the operator's declared scan scope, checked afresh for every one, exactly as the
		// core's own probes did before the sensor moved (Constitution II.3 item 3 and item 12).
		ScopedGuard: stackport.ScopedGuard{Check: func(a netip.Addr) error { return scope.Policy().Check(a) }, Dial: netTools.DialContext()},
	}
	h, err := newStackHost(st, host)
	if err != nil {
		return nil, err
	}
	s.AppCtx.AfterFactoryReset("The log of DNS comparisons", func() error { return stackport.Remove(dir, files) })
	return h, nil
}

// agentsStack is the Agents stack: the watch side of what the agents do. Only this file imports a stack (Constitution IV.6).
func agentsStack() stacksdk.Stack { return agents.New() }

// agentAlertIDs are the alert ids the Agents stack can publish; a test holds the core's table of words to them.
func agentAlertIDs() []string { return agents.AlertIDs() }

// newAgentsHost hosts the Agents stack over the core's own services, one adapter per port its manifest requires: the settings document (the
// `agents` key, unchanged), the alert sink (which words each alert and sends it to the connectors), the service's log, the data folder
// restricted to the one file the manifest lists, and the secrets scan (the folder walk stays in the core, so no file's bytes cross). The
// record file is removed by a factory reset, as the store it replaces was.
func (s *AppServices) newAgentsHost(policy agentsPolicy, notify func(ctx context.Context, message string) error) (*stackHost, error) {
	st := agentsStack()
	files := st.Manifest().Storage
	if _, err := claimedFiles(networkStack().Manifest(), st.Manifest()); err != nil {
		return nil, err
	}
	dir := s.AppCtx.RootDir()
	host := stacksdk.Host{
		Config:     stackport.Config[models.AgentsConfig]{Store: policy},
		Events:     agentwatch.Alerts{Notify: notify, OnError: func(err error) { logging.Warn("agent watch", "error", err.Error()) }},
		Log:        stackport.Log{Stack: st.Manifest().ID},
		DataDir:    stackport.DataDir{Dir: dir, Files: files},
		SecretScan: agentwatch.Scanner{Workspaces: s.workspaceDirs},
	}
	h, err := newStackHost(st, host)
	if err != nil {
		return nil, err
	}
	s.AppCtx.AfterFactoryReset("The agent security record", func() error { return stackport.Remove(dir, files) })
	return h, nil
}

// radioStack is the Radio stack: the sensors of wireless radios. Only this file imports a stack (Constitution IV.6). It reads the real /sys.
func radioStack() stacksdk.Stack { return radio.New(nil) }

// newRadioHost hosts the Radio stack over the core's own services, one adapter per port its manifest requires: the settings document (the
// `radio` key), the service's log, the data folder restricted to the one file the manifest lists (the database), and the vault, which derives
// a subkey per purpose from the master key so the stack never sees a key. The database is removed by a factory reset, which also replaces the key.
func (s *AppServices) newRadioHost(policy radioPolicy) (*stackHost, error) {
	st := radioStack()
	files := st.Manifest().Storage
	if _, err := claimedFiles(networkStack().Manifest(), agentsStack().Manifest(), st.Manifest()); err != nil {
		return nil, err
	}
	dir := s.AppCtx.RootDir()
	configDir := s.AppCtx.dataMgr.ConfigDir()
	h, err := newStackHost(st, stacksdk.Host{
		Config:  stackport.Config[models.RadioConfig]{Store: policy},
		Log:     stackport.Log{Stack: st.Manifest().ID},
		DataDir: stackport.DataDir{Dir: dir, Files: files},
		Vault:   stackport.Vault{Key: func() ([]byte, error) { return storage.GetMasterKey(configDir) }},
	})
	if err != nil {
		return nil, err
	}
	s.AppCtx.AfterFactoryReset("The radio database", func() error { return stackport.Remove(dir, files) })
	return h, nil
}

// pokedSwitch wakes a listener's loop after the setting changed, as the feature's own route did, so the socket opens or closes
// now and not at the next tick. A write that failed wakes nothing.
type pokedSwitch struct {
	stack.Switch
	poke func()
}

func withPoke(s stack.Switch, poke func()) stack.Switch { return pokedSwitch{Switch: s, poke: poke} }

func (p pokedSwitch) Set(enabled bool) error {
	if err := p.Switch.Set(enabled); err != nil {
		return err
	}
	p.poke()
	return nil
}

func agentActivitySwitch(p agentsPolicy) agentPart {
	return agentPart{p, func(c *models.AgentsConfig) *bool { return &c.ActivityOff }}
}

func secretsPassSwitch(p agentsPolicy) agentPart {
	return agentPart{p, func(c *models.AgentsConfig) *bool { return &c.SecretsPassOff }}
}

// agentPart is one "off" flag of the Agents policy read as a switch: on while the flag is not set.
type agentPart struct {
	store agentsPolicy
	off   func(*models.AgentsConfig) *bool
}

func (a agentPart) Enabled() bool {
	cfg := a.store.Get()
	return !*a.off(&cfg)
}

func (a agentPart) Set(enabled bool) error {
	return a.store.Update(func(c *models.AgentsConfig) error {
		*a.off(c) = !enabled
		return nil
	})
}

// observeTimeout bounds how long describing a module waits for its stack's status; a stack's status reads a counter or one small file.
const observeTimeout = 2 * time.Second

// observedModule is the registry's observer: it asks each hosted stack how a module is doing and gives back when it last ran and what is
// wrong with it, in the stack's own fixed words. A module no hosted stack reports, or a stack that is not hosted, says nothing.
func (n *netauditServices) observedModule(id string) (stack.Observed, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), observeTimeout)
	defer cancel()
	for _, h := range []*stackHost{n.Network, n.Agents, n.Radio} {
		if h == nil {
			continue
		}
		if st, ok := h.Module(ctx, id); ok {
			return stack.Observed{LastRun: st.LastRun, Problem: st.Problem}, true
		}
	}
	return stack.Observed{}, false
}

// stalledMonitors are the ids of the monitored parts that have stopped, for the health of the modules that own them.
func (s AppServices) stalledMonitors() []string {
	if s.netaudit == nil || s.netaudit.Monitors == nil {
		return nil
	}
	var ids []string
	for _, st := range s.netaudit.Monitors.Stalled() {
		ids = append(ids, st.ID)
	}
	return ids
}

// loginRecordReady says whether this program can read the system's record of failed logins and, when it cannot, what to do about it.
// A missing record is fine (nothing was ever recorded); a record the service may not open needs the utmp group.
func loginRecordReady(r loginlog.Reader) (bool, string) {
	err := r.Readable()
	switch {
	case err == nil:
		return true, ""
	case errors.Is(err, fs.ErrPermission):
		return false, "this program may not read the system's record of failed logins; add the utmp group to the service (see the service setup notes)"
	}
	return false, "the system's record of failed logins could not be read"
}
