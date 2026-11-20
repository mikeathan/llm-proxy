package storage

import "llm-proxy/models"

// newsroomView projects AppConfig.Newsroom: the Newsroom stack's zone and the module's switch. It is a view of its own, not part of UserSettings, so the
// ordinary settings save never touches it.
type newsroomView struct {
	mgr *DataManager
}

// Get returns a copy of the Newsroom settings; an absent section is the zero value, in which the module is on.
func (v *newsroomView) Get() models.NewsroomConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.NewsroomConfig {
		if cfg.Newsroom == nil {
			return models.NewsroomConfig{}
		}
		return *cfg.Newsroom
	})
}

// Update applies fn to a copy of the settings and saves the result.
func (v *newsroomView) Update(fn func(*models.NewsroomConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		var next models.NewsroomConfig
		if cfg.Newsroom != nil {
			next = *cfg.Newsroom
		}
		if err := fn(&next); err != nil {
			return err
		}
		cfg.Newsroom = &next
		return nil
	})
}
