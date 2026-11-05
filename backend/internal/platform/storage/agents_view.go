package storage

import "llm-proxy/models"

// agentsView projects AppConfig.Agents. It is a view of its own, not part of UserSettings, so the ordinary settings save never
// touches it.
type agentsView struct {
	mgr *DataManager
}

// Get returns a copy of the Agents policy; an absent section is the zero policy, in which every part is on.
func (v *agentsView) Get() models.AgentsConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.AgentsConfig {
		if cfg.Agents == nil {
			return models.AgentsConfig{}
		}
		return *cfg.Agents
	})
}

// Update applies fn to a copy of the policy and saves the result.
func (v *agentsView) Update(fn func(*models.AgentsConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		var next models.AgentsConfig
		if cfg.Agents != nil {
			next = *cfg.Agents
		}
		if err := fn(&next); err != nil {
			return err
		}
		cfg.Agents = &next
		return nil
	})
}
