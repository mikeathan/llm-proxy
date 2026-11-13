package storage

import "llm-proxy/models"

// radioView projects AppConfig.Radio. It is a view of its own, not part of UserSettings, so the ordinary settings save never touches it.
type radioView struct {
	mgr *DataManager
}

// Get returns a copy of the Radio policy; an absent section is the zero policy, in which the stack is off.
func (v *radioView) Get() models.RadioConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.RadioConfig {
		if cfg.Radio == nil {
			return models.RadioConfig{}
		}
		return *cfg.Radio
	})
}

// Update applies fn to a copy of the policy and saves the result.
func (v *radioView) Update(fn func(*models.RadioConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		var next models.RadioConfig
		if cfg.Radio != nil {
			next = *cfg.Radio
		}
		if err := fn(&next); err != nil {
			return err
		}
		cfg.Radio = &next
		return nil
	})
}
