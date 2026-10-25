package storage

import "llm-proxy/models"

// netauditView projects AppConfig.Netaudit. It is a view of its own, not part of UserSettings, so the ordinary
// settings save never touches it.
type netauditView struct {
	mgr *DataManager
}

// Get returns a copy of the network-audit policy; an absent section is the zero policy (scanning off).
func (v *netauditView) Get() models.NetauditConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.NetauditConfig {
		if cfg.Netaudit == nil {
			return models.NetauditConfig{}
		}
		return *cfg.Netaudit
	})
}

// Update applies fn to a copy of the policy and saves the result.
func (v *netauditView) Update(fn func(*models.NetauditConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		var next models.NetauditConfig
		if cfg.Netaudit != nil {
			next = *cfg.Netaudit
		}
		if err := fn(&next); err != nil {
			return err
		}
		cfg.Netaudit = &next
		return nil
	})
}
