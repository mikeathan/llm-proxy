package storage

import (
	"reflect"

	"llm-proxy/models"
)

// Package storage provides atomic JSON/YAML file stores and facade views over
// the single persisted AppConfig.
//
// SystemConfigView and UserSettingsView are facade projections over the single
// shared AppConfig store. They preserve the historical System()/Settings() API
// (Get/Update/OnChange) so the ~40 call sites and ApplySystemUpdate do not all
// churn at once, while guaranteeing the single-owner invariant: every read
// returns a deep-copied projection (never the live document), and every write
// updates and persists the complete merged document under one mutex.
// Concurrent system vs settings writes therefore serialize through the shared
// store rather than clobbering each other.

// onProjectionChange registers a listener on the shared AppConfig store that
// fires fn with the projection only when that projection actually changed.
// Because every AppConfig mutation notifies all views, filtering prevents a
// host-settings-only write from spuriously firing the system/settings
// subscribers (single-owner store, no cross-talk). All three facade views share
// this one implementation.
func onProjectionChange[T any](store *Store[models.AppConfig], project func(models.AppConfig) T, fn func(T)) {
	last := project(store.Get())
	store.OnChange(func(cfg models.AppConfig) {
		proj := project(cfg)
		if reflect.DeepEqual(last, proj) {
			return
		}
		last = proj
		fn(proj)
	})
}

// SystemConfigView projects AppConfig.Server/WorkspacesDir/Metrics as a
// SystemConfig. RunLogging is the canonical top-level AppConfig field, surfaced
// here as Server.RunLogging so the SystemConfig shape has one accessor.
type SystemConfigView struct {
	mgr *DataManager
}

func appConfigToSystem(cfg models.AppConfig) models.SystemConfig {
	return models.SystemConfig{
		Server: models.SystemServerConfig{
			AppServerConfig: models.AppServerConfig{
				Bind:            cfg.Server.Bind,
				ModelHost:       cfg.Server.ModelHost,
				IdleTimeoutSecs: cfg.Server.IdleTimeoutSecs,
				Environment:     cfg.Server.Environment,
				LogLevel:        cfg.Server.LogLevel,
			},
			RunLogging: cfg.RunLogging,
		},
		WorkspacesDir: cfg.WorkspacesDir,
		Metrics:       cfg.Metrics,
	}
}

func applySystemToAppConfig(cfg *models.AppConfig, sys models.SystemConfig) {
	cfg.Server.Bind = sys.Server.Bind
	cfg.Server.ModelHost = sys.Server.ModelHost
	cfg.Server.IdleTimeoutSecs = sys.Server.IdleTimeoutSecs
	cfg.Server.Environment = sys.Server.Environment
	cfg.Server.LogLevel = sys.Server.LogLevel
	cfg.RunLogging = sys.Server.RunLogging
	cfg.WorkspacesDir = sys.WorkspacesDir
	cfg.Metrics = sys.Metrics
}

// Get returns the system projection of the live AppConfig. It projects from a
// shallow read and deep-copies only the projection (not the whole document).
func (v *SystemConfigView) Get() models.SystemConfig {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.SystemConfig {
		return appConfigToSystem(*cfg)
	})
}

// Update applies a mutation to the system projection and persists the whole
// AppConfig. A failed callback leaves the merged document untouched.
func (v *SystemConfigView) Update(fn func(*models.SystemConfig) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		sys := appConfigToSystem(*cfg)
		if err := fn(&sys); err != nil {
			return err
		}
		applySystemToAppConfig(cfg, sys)
		return nil
	})
}

// OnChange registers a callback fired with the system projection only when that
// projection actually changed (see onProjectionChange rationale).
func (v *SystemConfigView) OnChange(fn func(models.SystemConfig)) {
	onProjectionChange(v.mgr.appConfigStore, appConfigToSystem, fn)
}

// UserSettingsView projects the user-tier fields (Local/Guardrails/
// ModelOverrides/Memory) of AppConfig as a UserSettings. RunLogging is the
// canonical top-level AppConfig field, surfaced here as RunOutput for the
// UserSettings view.
type UserSettingsView struct {
	mgr *DataManager
}

func appConfigToUserSettings(cfg models.AppConfig) models.UserSettings {
	return models.UserSettings{
		Local:          cfg.Local,
		Guardrails:     cfg.Guardrails,
		ModelOverrides: cfg.ModelOverrides,
		Memory:         cfg.Memory,
		RunOutput:      cfg.RunLogging,
	}
}

func applyUserSettingsToAppConfig(cfg *models.AppConfig, set models.UserSettings) {
	cfg.Local = set.Local
	cfg.Guardrails = set.Guardrails
	cfg.ModelOverrides = set.ModelOverrides
	cfg.Memory = set.Memory
	cfg.RunLogging = set.RunOutput
}

// Get returns the settings projection of the live AppConfig. It projects from a
// shallow read and deep-copies only the projection (not the whole document).
func (v *UserSettingsView) Get() models.UserSettings {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.UserSettings {
		return appConfigToUserSettings(*cfg)
	})
}

// Update applies a mutation to the settings projection and persists the whole
// AppConfig.
func (v *UserSettingsView) Update(fn func(*models.UserSettings) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		set := appConfigToUserSettings(*cfg)
		if err := fn(&set); err != nil {
			return err
		}
		applyUserSettingsToAppConfig(cfg, set)
		return nil
	})
}

// OnChange registers a callback fired with the settings projection only when that
// projection changed (see onProjectionChange rationale).
func (v *UserSettingsView) OnChange(fn func(models.UserSettings)) {
	onProjectionChange(v.mgr.appConfigStore, appConfigToUserSettings, fn)
}

// hostSettingsView projects AppConfig.Sandboxing as the HostSettings shape
// consumed by the Security Settings endpoint and bootstrap.
type hostSettingsView struct {
	mgr *DataManager
}

// Get returns the host projection of the live AppConfig. It projects from a
// shallow read and deep-copies only the projection (not the whole document).
func (v *hostSettingsView) Get() models.HostSettings {
	return GetProjected(v.mgr.appConfigStore, func(cfg *models.AppConfig) models.HostSettings {
		return models.HostSettings{Sandboxing: cfg.Sandboxing}
	})
}

func (v *hostSettingsView) Update(fn func(*models.HostSettings) error) error {
	return v.mgr.appConfigStore.Update(func(cfg *models.AppConfig) error {
		hs := models.HostSettings{Sandboxing: cfg.Sandboxing}
		if err := fn(&hs); err != nil {
			return err
		}
		cfg.Sandboxing = hs.Sandboxing
		return nil
	})
}

// OnChange registers a callback fired with the host projection only when it
// changed (see onProjectionChange rationale).
func (v *hostSettingsView) OnChange(fn func(models.HostSettings)) {
	onProjectionChange(v.mgr.appConfigStore, func(cfg models.AppConfig) models.HostSettings {
		return models.HostSettings{Sandboxing: cfg.Sandboxing}
	}, fn)
}
