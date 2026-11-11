package app

import (
	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/footprint"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
)

// The breach service key lives in the encrypted secrets store, beside the search providers' keys; it is only ever saved, cleared or
// used for a look, never answered back.
const (
	footprintSecretCategory = "netaudit"
	footprintSecretName     = "footprint_breach_key"
)

// footprintKey is the breach key in the secrets store.
type footprintKey struct{ app *AppContext }

func (k footprintKey) value() string {
	return k.app.Secrets().GetSecret(footprintSecretCategory, footprintSecretName)
}
func (k footprintKey) Saved() bool { return k.value() != "" }
func (k footprintKey) Set(key string) error {
	return k.app.Secrets().SetSecret(footprintSecretCategory, footprintSecretName, key)
}
func (k footprintKey) Clear() error {
	return k.app.Secrets().DeleteSecret(footprintSecretCategory, footprintSecretName)
}

// newFootprint builds what looks at the operator's domains and addresses outside the network. Every request goes through the guarded
// client, and only for a service the operator switched on; the audit reads the stored results and asks nothing.
func (s *AppServices) newFootprint(store netaudit.PolicyStore, netTools *tools.NetworkTools) *footprint.Manager {
	results := withReset(s.AppCtx, "The footprint results", persistence.NewFootprintStore(s.AppCtx.RootDir()))
	settings := footprint.NewSettings(store)
	key := footprintKey{app: s.AppCtx}
	service := footprint.NewService(footprint.Deps{
		Config: settings.Config, Store: results,
		Certs: footprint.CertLog{Opener: netTools}, Registry: &footprint.Registry{Opener: netTools},
		Mail: footprint.Mail{Opener: netTools}, Breaches: footprint.Breaches{Opener: netTools, Key: key.value},
		OnError: func(err error) { logging.Warn("network footprint", "error", err.Error()) },
	})
	return &footprint.Manager{Settings: settings, Service: service, Key: key}
}

// NetauditFootprint is what the audit's report reads: the footprint findings from stored results.
func (s *AppServices) NetauditFootprint() netaudit.FootprintSource {
	if s.netaudit == nil || s.netaudit.Footprint == nil {
		return nil
	}
	return s.netaudit.Footprint
}
