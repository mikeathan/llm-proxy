package app

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/internal/core/netaudit/routerhistory"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

// The router's SSH key lives in the encrypted secrets store, beside the other netaudit keys: it is only ever saved, cleared or used for a read,
// never answered back (Constitution I.15).
const (
	routerConnSecretCategory = "netaudit"
	routerConnSecretName     = "router_ssh_key"
)

// routerConnSecret is the router key in the secrets store.
type routerConnSecret struct{ app *AppContext }

func (k routerConnSecret) Value() string {
	return k.app.Secrets().GetSecret(routerConnSecretCategory, routerConnSecretName)
}
func (k routerConnSecret) Set(key string) error {
	return k.app.Secrets().SetSecret(routerConnSecretCategory, routerConnSecretName, key)
}
func (k routerConnSecret) Clear() error {
	return k.app.Secrets().DeleteSecret(routerConnSecretCategory, routerConnSecretName)
}

// routerConnSettings keeps the router's address, user and pinned key in the Network settings.
type routerConnSettings struct{ store netaudit.PolicyStore }

func (s routerConnSettings) Get() (string, string, string) {
	c := s.store.Get()
	return c.RouterConnAddress, c.RouterConnUser, c.RouterConnHostKey
}

func (s routerConnSettings) Set(address, user, hostKey string) error {
	return s.store.Update(func(c *models.NetauditConfig) error {
		c.RouterConnAddress, c.RouterConnUser, c.RouterConnHostKey = address, user, hostKey
		return nil
	})
}

// routerConnParts are the router's connection table (read on the operator's press, and once a night by the audit's own code while the watch is
// on) with the watch's switch and history.
type routerConnParts struct {
	Manager *routerconn.Manager
	Config  *netaudit.RouterConnConfig
	History *routerhistory.History
}

// newRouterConn builds what reads the router's connection table. The key goes with the factory reset like every other secret of the module,
// and so does the history; the last read is kept in memory only. While the watch is on, each read is added to the history before it is kept,
// which marks the places that are new.
func (s *AppServices) newRouterConn(store netaudit.PolicyStore, inventory *netaudit.Inventory) routerConnParts {
	secret := routerConnSecret{app: s.AppCtx}
	s.AppCtx.AfterFactoryReset("The router's SSH key", secret.Clear)
	config := netaudit.NewRouterConnConfig(store)
	file := withReset(s.AppCtx, "What each device connected to", persistence.NewRouterHistoryStore(s.AppCtx.RootDir()))
	history, err := routerhistory.New(file)
	if err != nil {
		logging.Warn("network router connections", "error", err.Error())
		history = routerhistory.NewEmpty(file)
	}
	manager := routerconn.New(routerconn.Deps{
		Settings: routerConnSettings{store: store}, Secret: secret,
		Read: routerssh.ReadTable, Probe: routerssh.ProbeHostKey,
		Owners: func() routerconn.Owners {
			if owners := s.netaudit.currentOwners(); owners != nil {
				return func(a netip.Addr) (models.TrafficOwner, bool) { return owners(a) }
			}
			return nil
		},
		Watching: config.Enabled,
		Observe:  routerConnObserver(config.Enabled, history, inventory, time.Now),
	})
	return routerConnParts{Manager: manager, Config: config, History: history}
}

// routerConnObserver adds a read to the history while the watch is on. A device is known by its hardware address when the inventory has
// one for its address, else by the address itself. A history that cannot be written is logged and never stops the read.
func routerConnObserver(enabled func() bool, history *routerhistory.History, inventory *netaudit.Inventory, now func() time.Time) func(*models.RouterConnections) {
	return func(read *models.RouterConnections) {
		if !enabled() {
			return
		}
		macs := map[string]string{}
		if inventory != nil {
			if inv, err := inventory.View(); err == nil {
				for _, d := range inv.Devices {
					if d.IP != "" && d.MAC != "" {
						macs[d.IP] = d.MAC
					}
				}
			}
		}
		key := func(ip string) string {
			if mac := macs[ip]; mac != "" {
				return mac
			}
			return "ip:" + ip
		}
		if err := history.Observe(read, key, now()); err != nil {
			logging.Warn("network router connections", "error", err.Error())
		}
	}
}

// routerConnWatch is what the nightly report reads of the watch: the report's own code asks it to read the router once, so no tool of an
// agent, and nothing an agent writes, reaches the router, its user or its key (Constitution I.15).
type routerConnWatch struct {
	config  *netaudit.RouterConnConfig
	manager *routerconn.Manager
	history *routerhistory.History
	// ready says whether the guardrails still allow the local network, asked at each read as the web checks do.
	ready func() (bool, string)
}

func (w routerConnWatch) Enabled() bool { return w.config.Enabled() }

// Refresh reads the router once. A setup that is not finished, or a router that was just read, is not a failure: there was nothing to do. A
// guardrail that has since closed the local network is one: nothing is sent and the report says why.
func (w routerConnWatch) Refresh(ctx context.Context) error {
	if w.ready != nil {
		if ok, why := w.ready(); !ok {
			return errors.New(why)
		}
	}
	_, err := w.manager.Read(ctx)
	return mapRefreshError(err)
}

// mapRefreshError says that a setup that is not finished, or a router that was just read, is not a failure of the nightly read.
func mapRefreshError(err error) error {
	if errors.Is(err, routerconn.ErrNotSet) || errors.Is(err, routerconn.ErrBusy) {
		return netaudit.ErrRouterConnSkipped
	}
	return err
}

// Signals are what is new in the history while the watch is on, and nothing while it is off.
func (w routerConnWatch) Signals(now time.Time) []routerhistory.Signal {
	if !w.config.Enabled() {
		return nil
	}
	return w.history.Signals(now)
}

// NetauditRouterConn is what the run reporter reads of the router watch; nil until the services are wired.
func (s AppServices) NetauditRouterConn() netaudit.RouterConnSource {
	if s.netaudit == nil || s.netaudit.RouterConn == nil || s.netaudit.RouterConnConfig == nil || s.netaudit.RouterHistory == nil {
		return nil
	}
	return routerConnWatch{config: s.netaudit.RouterConnConfig, manager: s.netaudit.RouterConn, history: s.netaudit.RouterHistory, ready: s.netaudit.RouterConnReady}
}

// routerConnReady says whether the router watch can be switched on: the local network is allowed and the router's setup is finished
// (address, user, key and the router's own key pinned).
func routerConnReady(lan func() (bool, string), manager *routerconn.Manager) (bool, string) {
	if ok, why := lan(); !ok {
		return false, why
	}
	if manager == nil {
		return false, "The router's connection table is not available until the service has started."
	}
	if v := manager.View(); v.Address == "" || v.User == "" || !v.KeySaved || v.Fingerprint == "" {
		return false, "Finish the router's setup on the Traffic tab first: its address, the key and the router's own key."
	}
	return true, ""
}
