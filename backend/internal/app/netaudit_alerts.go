package app

import (
	"context"
	"net"
	"net/netip"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/names"
	"llm-proxy/internal/platform/logging"
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
}

func (s *AppServices) newAlerting(deps alertingDeps) alerting {
	root := s.AppCtx.RootDir()
	alertStore := withReset(s.AppCtx, "The log of the Network module's alerts", persistence.NewNetworkAlertStore(root))
	log := netaudit.NewAlertLog(alertStore, time.Now, func(err error) { logging.Warn("network alerts", "error", err.Error()) })

	known := deps.Tracking
	digest := s.newDigest(known.Inventory, known.History, known.Summary, s.alertNotifier(log, models.NetworkAlertSummary))
	presenceConfig := netaudit.NewPresenceConfig(deps.Store)
	presence := s.newPresence(deps.Service, known.Inventory, presenceConfig, presenceHooks{
		OnLook: lookHooks(digest, known.Names),
		Notify: s.alertNotifier(log, models.NetworkAlertPresence),
	})

	decoyStore := withReset(s.AppCtx, "The record of connections to decoy services", persistence.NewDecoyTouchStore(root))
	decoyConfig := netaudit.NewDecoyConfig(deps.Store)
	return alerting{
		Log:            log,
		Presence:       presence,
		PresenceConfig: presenceConfig,
		Decoys:         s.newDecoys(decoyConfig, decoyStore, log, deps.Scope),
		DecoyConfig:    decoyConfig,
		DecoyStore:     decoyStore,
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

// newDecoys builds the decoy services. A connection counts only from a device inside the declared scan networks and not from
// this computer itself, whose own audit and sweep must never look like an intruder. The alert goes to every connector.
func (s *AppServices) newDecoys(config *netaudit.DecoyConfig, store *persistence.DecoyTouchStore, alerts *netaudit.AlertLog,
	scope *netaudit.Scope) *decoy.Manager {
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
