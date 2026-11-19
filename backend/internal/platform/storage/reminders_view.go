package storage

import "llm-proxy/models"

// remindersView projects AppConfig.Reminders for the Reminders stack: the owner's skipped connectors and zone, and the module's switch. The settings
// form writes the same section (UserSettings), so its save keeps the switch as it was (applySettingsFields), and the switch is changed only here.
type remindersView struct {
	mgr *DataManager
}

// Get returns a copy of the reminders settings; an absent section is the zero value, in which the module is on.
func (v *remindersView) Get() models.RemindersConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.RemindersConfig {
		return cfg.Reminders.Effective()
	})
}

// Update applies fn to a copy of the settings and saves the result.
func (v *remindersView) Update(fn func(*models.RemindersConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		next := cfg.Reminders.Effective()
		if err := fn(&next); err != nil {
			return err
		}
		cfg.Reminders = &next
		return nil
	})
}
