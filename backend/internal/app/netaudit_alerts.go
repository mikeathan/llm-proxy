package app

import (
	"context"
	"llm-proxy/internal/platform/loginlog"
	"net/netip"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/names"
	"llm-proxy/internal/core/netaudit/outsidelookup"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// The Network module's alerts: what raises them (the presence check, the decoy services, the weekly summary) and how they
// go out, through the alert log to every connector.

// alertingDeps are what the alert sources are built from.
type alertingDeps struct {
	Store    netaudit.PolicyStore
	Scope    *netaudit.Scope
	Service  *netaudit.Service
	Tracking tracking
	// PhoneHome is the watch of the names devices ask for; the router log's listener hands it the DNS query lines.
	PhoneHome phoneHome
}

// notifyFunc sends one alert message; it fails when the message could not be delivered, so the alert stays pending.
type notifyFunc = func(ctx context.Context, message string) error

// alerting is the alert log and the things that write to it.
type alerting struct {
	Log            *netaudit.AlertLog
	Presence       *netaudit.Presence
	PresenceConfig *netaudit.PresenceConfig
	Decoys         *decoy.Manager
	DecoyConfig    *netaudit.DecoyConfig
	DecoyStore     *persistence.DecoyTouchStore

	RouterLogConfig *netaudit.RouterLogConfig
	RouterLog       *routerlog.Manager
	// Blocks counts the refused attempts the router's log reports and writes them to the history on its own timer.
	Blocks *netaudit.Blocks
	// RouterWatch tells of a burst of failed router sign-ins or Wi-Fi disconnects.
	RouterWatch *netaudit.RouterWatch
	// LoginWatch tells of a burst of failed logins to this computer; LoginWatchConfig is its switch.
	LoginWatch       *netaudit.LoginWatch
	LoginWatchConfig *netaudit.LoginWatchConfig
	// FollowUp scans a device that just arrived, once, when its switch is on; FollowUpConfig is that switch.
	FollowUp       *netaudit.FollowUp
	FollowUpConfig *netaudit.FollowUpConfig
}

func (s *AppServices) newAlerting(deps alertingDeps) alerting {
	root := s.AppCtx.RootDir()
	alertStore := withReset(s.AppCtx, "The log of the Network module's alerts", persistence.NewNetworkAlertStore(root))
	log := netaudit.NewAlertLog(alertStore, time.Now, func(err error) { logging.Warn("network alerts", "error", err.Error()) })

	known := deps.Tracking
	digest := s.newDigest(known.Inventory, known.History, known.Summary, s.alertNotifier(log, models.NetworkAlertSummary))
	presenceConfig := netaudit.NewPresenceConfig(deps.Store)
	followUpConfig := netaudit.NewFollowUpConfig(deps.Store)
	followUp := newFollowUp(deps.Service, followUpConfig, s.alertNotifier(log, models.NetworkAlertFollowUp))
	presence := s.newPresence(deps.Service, known.Inventory, presenceConfig, presenceHooks{
		OnLook: lookHooks(digest, known.Names, followUp),
		Notify: s.alertNotifier(log, models.NetworkAlertPresence),
	})

	decoyStore := withReset(s.AppCtx, "The record of connections to decoy services", persistence.NewDecoyTouchStore(root))
	decoyConfig := netaudit.NewDecoyConfig(deps.Store)
	routerLogConfig := netaudit.NewRouterLogConfig(deps.Store)
	routerWatch := netaudit.NewRouterWatch(netaudit.RouterWatchDeps{
		Enabled: routerLogConfig.Enabled,
		Notify:  s.alertNotifier(log, models.NetworkAlertRouter),
		OnError: func(err error) { logging.Warn("network router log alerts", "error", err.Error()) },
	})
	loginWatchConfig := netaudit.NewLoginWatchConfig(deps.Store)
	loginWatch := netaudit.NewLoginWatch(netaudit.LoginWatchDeps{
		Read:    loginAttempts(loginlog.Reader{Path: loginlog.DefaultPath}),
		Enabled: loginWatchConfig.Enabled,
		Notify:  s.alertNotifier(log, models.NetworkAlertLogin),
		OnError: func(err error) { logging.Warn("network login watch", "error", err.Error()) },
		Cap:     loginlog.MaxRecords,
	})
	blocks := netaudit.NewBlocks(known.History, func(err error) { logging.Warn("network router blocks", "error", err.Error()) })
	return alerting{
		Log:            log,
		Presence:       presence,
		PresenceConfig: presenceConfig,
		Decoys:         s.newDecoys(decoyConfig, decoyStore, log, deps.Scope, cachedOwnAddress(deps.Scope, ownAddressTTL, time.Now), decoyAlertText(known.Inventory.View, time.Now)),
		DecoyConfig:    decoyConfig,
		DecoyStore:     decoyStore,

		RouterLogConfig:  routerLogConfig,
		RouterLog:        s.newRouterLog(routerLogDeps{Config: routerLogConfig, Inventory: known.Inventory, Suggester: known.Names, Presence: presence, Blocks: blocks, Router: routerWatch, Query: deps.PhoneHome.Intake.Observe}),
		Blocks:           blocks,
		RouterWatch:      routerWatch,
		LoginWatch:       loginWatch,
		LoginWatchConfig: loginWatchConfig,
		FollowUp:         followUp,
		FollowUpConfig:   followUpConfig,
	}
}

// newFollowUp builds the scan on arrival. It scans through the scan service, so the declared scope and the one-scan-at-a-time
// rule apply; the mode is the ordinary service scan, never the deep check; the alert has its own kind.
func newFollowUp(service *netaudit.Service, config *netaudit.FollowUpConfig, notify notifyFunc) *netaudit.FollowUp {
	return netaudit.NewFollowUp(netaudit.FollowUpDeps{
		Enabled: config.Enabled,
		Scan: func(ctx context.Context, addr string) (netaudit.Result, error) {
			return service.Scan(ctx, netaudit.Request{Targets: []string{addr}, Mode: netaudit.ModeServices, Unattended: true})
		},
		Notify:  notify,
		OnError: func(err error) { logging.Warn("network follow-up scan", "error", err.Error()) },
	})
}

// outsideLookup is the opt-in look at what the internet can reach: its setting, and the check the router check runs.
type outsideLookup struct {
	Config   *netaudit.OutsideLookupConfig
	Exposure *netaudit.Exposure
}

// newOutsideLookup builds the setting and the check. The one request goes through opener (the guarded network client), to the
// service the setting names, read on every look. The router's internet address comes with the router sensor's report.
func newOutsideLookup(store netaudit.PolicyStore, opener outsidelookup.Opener) outsideLookup {
	config := netaudit.NewOutsideLookupConfig(store)
	client := outsidelookup.Client{Opener: opener, BaseURL: config.URL}
	return outsideLookup{Config: config, Exposure: netaudit.NewExposure(config.Enabled, client, time.Now).WithEnteredAddress(config.Address)}
}

// gatewayTTL is how long the router's address is remembered before the routing table is read again.
const gatewayTTL = 30 * time.Second

// routerLogDeps are what the router log's listener is built from. Query takes a DNS query line the listener read.
type routerLogDeps struct {
	Config    *netaudit.RouterLogConfig
	Inventory *netaudit.Inventory
	Suggester *names.Suggester
	Presence  *netaudit.Presence
	Blocks    *netaudit.Blocks
	Router    *netaudit.RouterWatch
	Query     func(routerlog.Query, time.Time)
}

// newRouterLog builds the listener for the router's log. A datagram counts only from the default gateway. A device message can
// only wake the presence check when the device list does not know the hardware address in it, and a firewall drop is only
// counted (protocol and port, into the day's history); a DNS query line goes to the phone-home watch; none starts anything else (Constitution I.8).
func (s *AppServices) newRouterLog(d routerLogDeps) *routerlog.Manager {
	return routerlog.New(routerlog.Deps{
		Enabled: d.Config.Enabled,
		Gateway: cachedGateway(neighborSource{reader: neighbors.NewReader()}, gatewayTTL, time.Now),
		Device:  cachedDevices(d.Inventory.View, deviceListTTL, time.Now),
		Suggest: d.Suggester.Offer,
		Drop:    d.Blocks.Add,
		Query:   d.Query,
		Watch:   d.Router.Add,
		Wake:    d.Presence.Wake,
		OnError: func(err error) { logging.Warn("network router log", "error", err.Error()) },
	})
}

// cachedGateway reads the default gateway from the neighbour source and keeps it for ttl, so a burst of datagrams does not read
// the routing table once each.
func cachedGateway(src netaudit.NeighborSource, ttl time.Duration, now func() time.Time) func() netip.Addr {
	var (
		mu   sync.Mutex
		addr netip.Addr
		read time.Time
	)
	return func() netip.Addr {
		mu.Lock()
		defer mu.Unlock()
		if !read.IsZero() && now().Sub(read) < ttl {
			return addr
		}
		table, err := src.Neighbors(context.Background())
		read = now()
		addr = netip.Addr{}
		if err == nil {
			addr = table.Gateway
		}
		return addr
	}
}

// deviceListTTL is how long the device list is remembered between lookups for the router log. Leases and Wi-Fi joins come in
// bursts, and reading the whole list from disk for each would let a stream of forged lines keep the disk and the decoder busy.
const deviceListTTL = 5 * time.Second

// cachedDevices answers "is this hardware address in the device list, and has the operator named it" from a copy of the list
// that is at most ttl old. A stale answer is harmless: the worst case is a wake that the pacing already limits, or a name offer
// for a device that was named a moment ago, which the name store drops at its next look.
func cachedDevices(view func() (models.DeviceInventory, error), ttl time.Duration, now func() time.Time) func(string) (bool, bool, error) {
	var (
		mu   sync.Mutex
		read time.Time
		list map[string]bool // hardware address -> named
	)
	return func(mac string) (known, named bool, err error) {
		mu.Lock()
		defer mu.Unlock()
		if list == nil || now().Sub(read) >= ttl {
			inv, err := view()
			if err != nil {
				return false, false, err
			}
			list = make(map[string]bool, len(inv.Devices))
			for _, d := range inv.Devices {
				list[d.MAC] = d.Name != ""
			}
			read = now()
		}
		named, known = list[mac]
		return known, named, nil
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
func (s *AppServices) alertNotifier(alerts *netaudit.AlertLog, kind models.NetworkAlertKind) notifyFunc {
	return func(ctx context.Context, message string) error {
		return alerts.Send(ctx, kind, message, s.sendToConnectors)
	}
}

// newDigest builds the weekly summary, which the presence look sends when it is due.
func (s *AppServices) newDigest(inventory *netaudit.Inventory, history *netaudit.History, summary *netaudit.SummaryConfig,
	notify notifyFunc) *netaudit.WeeklyDigest {
	return netaudit.NewWeeklyDigest(netaudit.DigestDeps{
		Enabled: summary.Enabled, View: inventory.View, History: history, Notify: notify,
		OnError: func(err error) { logging.Warn("network weekly summary", "error", err.Error()) },
	})
}

// lookHooks is the work that rides on each presence look, in one place: asking unnamed devices their names, sending the weekly
// summary when it is due, and queueing a scan of the devices that just arrived (which runs from its own loop, so the look never
// waits on it). None adds a sweep; all stop with the context.
func lookHooks(digest *netaudit.WeeklyDigest, suggester *names.Suggester, followUp *netaudit.FollowUp) func(context.Context, netaudit.Snapshot) {
	name := nameUnnamed(suggester)
	return func(ctx context.Context, snap netaudit.Snapshot) {
		name(ctx, snap)
		digest.Tick(ctx)
		followUp.Offer(snap)
	}
}

// presenceHooks are what rides on the presence check: the work done on each look, and where its alert goes.
type presenceHooks struct {
	OnLook func(context.Context, netaudit.Snapshot)
	Notify notifyFunc
}

// newPresence builds the presence check. It sweeps through the scan service, so the declared scope and the one-scan-at-a-time
// rule apply; the alert goes to every connector, in counts and fixed words, and no model is involved.
func (s *AppServices) newPresence(service *netaudit.Service, inventory *netaudit.Inventory, config *netaudit.PresenceConfig,
	hooks presenceHooks) *netaudit.Presence {
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

// decoyAlertText words the decoy alert, saying so when a visitor is a device that is new on the network. A device list that cannot
// be read leaves the phrase out; the alert itself never waits on it.
func decoyAlertText(view func() (models.DeviceInventory, error), now func() time.Time) func([]string) string {
	return func(sources []string) string {
		inv, err := view()
		if err != nil {
			return netaudit.DecoyMessage(sources)
		}
		return netaudit.DecoyAlertMessage(sources, inv, now())
	}
}

// newDecoys builds the decoy services. A connection counts only from a device inside the declared scan networks and not from
// this computer itself (own), whose own audit and sweep must never look like an intruder. own reads the addresses the scan scope
// reads, with its fallback for a service that may not list interfaces, so the guard holds where netlink is refused. The alert,
// worded by text, goes to every connector.
func (s *AppServices) newDecoys(config *netaudit.DecoyConfig, store *persistence.DecoyTouchStore, alerts *netaudit.AlertLog,
	scope *netaudit.Scope, own func(netip.Addr) bool, text func([]string) string) *decoy.Manager {
	return decoy.New(decoy.Deps{
		Specs:   decoy.Specs,
		Enabled: config.Enabled,
		InScope: func(a netip.Addr) bool { return scope.Policy().Check(a) == nil },
		IsOwn:   own,
		Store:   store,
		Notify: func(ctx context.Context, sources []string) error {
			return alerts.Send(ctx, models.NetworkAlertDecoy, text(sources), s.sendToConnectors)
		},
		OnError: func(err error) { logging.Warn("network decoys", "error", err.Error()) },
	})
}

// loginAttempts adapts the failed-login reader to the watch's own type, so the watch imports no platform package.
func loginAttempts(r loginlog.Reader) func(after time.Time) ([]netaudit.LoginAttempt, error) {
	return func(after time.Time) ([]netaudit.LoginAttempt, error) {
		got, err := r.Attempts(after)
		if err != nil {
			return nil, err
		}
		out := make([]netaudit.LoginAttempt, len(got))
		for i, a := range got {
			out[i] = netaudit.LoginAttempt{At: a.At, From: a.From}
		}
		return out, nil
	}
}
