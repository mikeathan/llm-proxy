package app

import (
	"errors"
	"fmt"
	"io/fs"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/platform/loginlog"
	"llm-proxy/models"
)

// The registry of stacks and modules (ADR-0028) is described here, once, in the composition root. It owns no setting: each module
// is a Switch over the setting that already holds it, doing what that feature's own route did when it was switched.

// agentsPolicy is the Agents stack's stored switches: *storage's agents view.
type agentsPolicy interface {
	Get() models.AgentsConfig
	Update(func(*models.AgentsConfig) error) error
}

// stackDeps are what the registry is built from.
type stackDeps struct {
	Network *netauditServices
	Agents  agentsPolicy
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
	reg, err := stack.New(stackDefinitions(), described(append(networkModules(d), agentModules(d)...)))
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
		{ID: "login_watch", Title: "Failed logins, this computer", Purpose: "Tells you when someone keeps failing to log in here.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsNothing,
			Switch: n.LoginWatchConfig, Monitors: []string{netaudit.MonitorLoginWatch}, Ready: loginReady},
		{ID: "known_bad", Title: "Known-bad names", Purpose: "Warns when a device asks for a host that serves malware.", Stack: stack.Network, Layer: "listen", Sends: stack.SendsNothing,
			Needs: []string{"phone_home"}, Switch: n.KnownBadConfig, Ready: n.KnownBadList.Ready},
		{ID: "scan_on_arrival", Title: "Scan on arrival", Purpose: "Scan a new device once.", Stack: stack.Network, Layer: "test", Sends: stack.SendsLAN, StepUp: true,
			Needs: []string{"presence"}, Switch: n.FollowUpConfig},
		{ID: "web_checks", Title: "Web and certificate checks, nightly", Purpose: "Looks at the web page and certificate of every device the audit finds.", Stack: stack.Network, Layer: "test", Sends: stack.SendsLAN, StepUp: true,
			Ready: lan, Switch: n.WebChecksConfig, Monitors: []string{netaudit.MonitorWebChecks}},
		{ID: "outside_lookup", Title: "Internet view", Purpose: "Ports the internet can reach.", Stack: stack.Network, Layer: "outside", Sends: stack.SendsInternet, StepUp: true,
			Switch: n.OutsideLookupConfig, Ready: internet},
		{ID: "dns_check", Title: "Router DNS check", Purpose: "Is the router answering honestly?", Stack: stack.Network, Layer: "outside", Sends: stack.SendsInternet, StepUp: true,
			Switch: dnsCheckSwitch{n.DNSCheckConfig}, Monitors: []string{netaudit.MonitorDNS}, Ready: both},
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

// presenceSwitch turns the presence check on or off without touching the interval the operator chose.
type presenceSwitch struct{ *netaudit.PresenceConfig }

func (p presenceSwitch) Enabled() bool { return p.View().Enabled }

func (p presenceSwitch) Set(enabled bool) error { return p.SetEnabled(enabled) }

// dnsCheckSwitch is the DNS check's switch alone; its names, server and interval stay on its own card.
type dnsCheckSwitch struct{ *netaudit.DNSCheckConfig }

func (d dnsCheckSwitch) Enabled() bool { return d.Settings().Enabled }

func (d dnsCheckSwitch) Set(enabled bool) error { return d.SetEnabled(enabled) }

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
