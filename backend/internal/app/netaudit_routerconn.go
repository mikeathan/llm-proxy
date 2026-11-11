package app

import (
	"net/netip"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/routerconn"
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

// newRouterConn builds what reads the router's connection table on the operator's press. The key goes with the factory reset like every other
// secret of the module; the last read is kept in memory only.
func (s *AppServices) newRouterConn(store netaudit.PolicyStore) *routerconn.Manager {
	secret := routerConnSecret{app: s.AppCtx}
	s.AppCtx.AfterFactoryReset("The router's SSH key", secret.Clear)
	return routerconn.New(routerconn.Deps{
		Settings: routerConnSettings{store: store}, Secret: secret,
		Read: routerssh.ReadTable, Probe: routerssh.ProbeHostKey,
		Owners: func() routerconn.Owners {
			if owners := s.netaudit.currentOwners(); owners != nil {
				return func(a netip.Addr) (models.TrafficOwner, bool) { return owners(a) }
			}
			return nil
		},
	})
}
