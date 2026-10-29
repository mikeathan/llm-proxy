package app

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"

	"llm-proxy/internal/platform/env"
	"llm-proxy/models"
)

// Tier 1: System
func (s *AppContext) GetSystem() models.SystemConfig {
	return s.dataMgr.System().Get()
}

func (s *AppContext) UpdateSystem(fn func(*models.SystemConfig)) error {
	return s.dataMgr.System().Update(func(sys *models.SystemConfig) error {
		fn(sys)
		return nil
	})
}

func (s *AppContext) SetGPUConfig(cfg models.GPUConfig) {
	s.gpuConfig = cfg
	s.refreshMetricsService()
}

func (s *AppContext) GPUConfig() models.GPUConfig {
	return s.gpuConfig
}

func (s *AppContext) RunLoggingEnabled() bool {
	if s.cliEnableRuns {
		return true
	}
	sys := s.GetSystem()
	if sys.Server.RunLogging != nil {
		return sys.Server.RunLogging.Enabled
	}
	return false
}

func (s *AppContext) HostSettings() models.HostSettings {
	settings := s.dataMgr.HostSettings().Get()
	// Inject runtime functional state
	settings.Sandboxing.Functional = (s.terminal != nil)
	// Decode-only presence marker: never part of the API surface.
	settings.Sandboxing.SectionPresent = false
	// Inject the read-time Effective projection of OS enforcement (SPEC-006
	// §II.7.4): what the host actually applies, never persisted.
	if s.sandboxProv != nil {
		eff := s.sandboxProv.Effective()
		settings.Effective = &models.SandboxEffective{
			Filesystem: models.SandboxSurface{
				Mechanism: string(eff.Filesystem.Mechanism),
				Reason:    eff.Filesystem.Reason,
			},
			Network: models.SandboxSurface{
				Mechanism: string(eff.Network.Mechanism),
				Reason:    eff.Network.Reason,
			},
			Provider: eff.Provider,
		}
	}
	return settings
}

func (s *AppContext) UpdateHostSettings(settings models.HostSettings) error {
	// Functional + Effective are runtime projections — never persist a read echo
	// of them (the PUT body may round-trip the GET shape verbatim). Both are
	// recomputed on every GET from live runtime state.
	settings.Sandboxing.Functional = false
	settings.Effective = nil
	return s.dataMgr.HostSettings().Update(func(hs *models.HostSettings) error {
		*hs = settings
		return nil
	})
}

func (s *AppContext) ServiceCredentials() (id, secret string) {
	return os.Getenv("SERVICE_CLIENT_ID"), os.Getenv("SERVICE_CLIENT_SECRET")
}

func (s *AppContext) Environment() map[string]string {
	return s.GetSystem().Server.Environment
}

// ApplySystemUpdate applies a batched system/settings/registry/env mutation in a
// single transactionally-fenced sequence. It is the authoritative admin entry
// point for infrastructure configuration changes.
func (s *AppContext) ApplySystemUpdate(ctx context.Context, req models.SystemUpdatePayload) error {
	// Validate boundary input once, before any persistence happens, so an
	// invalid search config is rejected before either registry-update branch.
	if err := validateSystemUpdate(req); err != nil {
		return err
	}

	// 1. Update Infrastructure (SystemConfig)
	err := s.dataMgr.System().Update(func(sys *models.SystemConfig) error {
		s.applyServerFields(sys, req)
		applyGPUFields(sys, req)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to save system config: %w", err)
	}

	if s.metrics != nil && req.GPUSmoothingAlpha > 0 {
		s.metrics.SetSmoothingAlpha(req.GPUSmoothingAlpha)
	}

	// 1.5 Update Settings
	err = s.dataMgr.Settings().Update(func(set *models.UserSettings) error {
		applySettingsFields(set, req)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}

	// 2. Registry and Infrastructure updates (Sync handled by subscribers)
	if err := s.applyRegistryUpdate(req); err != nil {
		return err
	}

	// 5. Update Environment Variables (.env)
	applyServiceCredentials(req)

	// 7. Sync Guardrails
	if req.Guardrails != nil {
		if err := s.UpdateSettings(func(set *models.UserSettings) {
			set.Guardrails = req.Guardrails
		}); err != nil {
			return fmt.Errorf("failed to sync guardrails: %w", err)
		}
	}

	return nil
}

// validateSystemUpdate rejects an invalid search, scheduler, agent or reminders block.
func validateSystemUpdate(req models.SystemUpdatePayload) error {
	if err := req.Agent.Validate(); err != nil {
		return err
	}
	if req.Search != nil {
		if err := req.Search.Validate(); err != nil {
			return err
		}
	}
	if req.Scheduler != nil {
		if err := req.Scheduler.Validate(); err != nil {
			return err
		}
	}
	return req.Reminders.Validate()
}

// applyServerFields copies the server-level fields a request sets onto sys.
func (s *AppContext) applyServerFields(sys *models.SystemConfig, req models.SystemUpdatePayload) {
	if req.Bind != "" {
		sys.Server.Bind = req.Bind
	}
	if req.WorkspacesDir != "" {
		sys.WorkspacesDir = req.WorkspacesDir
		s.SetWorkspacesDir(req.WorkspacesDir)
	}
	if req.ModelHost != "" {
		sys.Server.ModelHost = req.ModelHost
	}
	if req.IdleTimeoutSecs != nil {
		// Pointer field: an explicit 0/-1 ("never stop the model") is a
		// valid value, not an "unset" sentinel. -1 (or <= 0) means the
		// model is never reaped (lifecycle.go); only -1 survives the
		// defaults-merge on reload (0 is treated as "unset" there).
		sys.Server.IdleTimeoutSecs = *req.IdleTimeoutSecs
	}
	if req.Environment != nil {
		sys.Server.Environment = req.Environment
	}
	if req.RunLogging != nil {
		sys.Server.RunLogging = req.RunLogging
	}
}

// applyGPUFields syncs the GPU configuration (kept inside the system transaction for persistence).
func applyGPUFields(sys *models.SystemConfig, req models.SystemUpdatePayload) {
	if req.GPUProvider != "" {
		sys.Metrics.GPU.Provider = req.GPUProvider
	}
	if req.GPUBinary != "" {
		sys.Metrics.GPU.Binary = req.GPUBinary
	}
	if req.GPUIndex != nil {
		sys.Metrics.GPU.Index = *req.GPUIndex
	}
	if req.GPUSysfsPath != "" {
		sys.Metrics.GPU.SysfsPath = req.GPUSysfsPath
	}
	if req.GPUSampleIntervalSec > 0 {
		sys.Metrics.GPUSampleIntervalSec = req.GPUSampleIntervalSec
	}
	if req.GPUSmoothingAlpha > 0 {
		sys.Metrics.GPUSmoothingAlpha = req.GPUSmoothingAlpha
	}
}

// applySettingsFields copies the user-settings blocks a request sets onto set.
func applySettingsFields(set *models.UserSettings, req models.SystemUpdatePayload) {
	if req.DefaultArgs != nil {
		set.Local.DefaultArgs = req.DefaultArgs
	}
	if req.Scheduler != nil {
		set.Scheduler = req.Scheduler
	}
	if req.Reminders != nil {
		set.Reminders = req.Reminders
	}
	if req.Agent != nil {
		set.Agent = req.Agent
	}
	if req.Memory != nil {
		if set.Memory == nil {
			set.Memory = new(models.DefaultMemoryConfig())
		}
		set.Memory.AssistantHot = new(req.Memory.AssistantHot)
		set.Memory.AutomationHot = new(req.Memory.AutomationHot)
	}
	if local, ok := req.Providers["local"]; ok {
		// We allow clearing these fields by removing the != "" check
		set.Local.LlamaServerBinary = local.LlamaServerBinary
		set.Local.ModelDir = local.ModelDir
		if local.DefaultArgs != nil {
			set.Local.DefaultArgs = local.DefaultArgs
		}
	}
}

// applyRegistryUpdate writes the providers and registry-level fields of a request in one registry update.
// A request that carries neither leaves the registry untouched.
func (s *AppContext) applyRegistryUpdate(req models.SystemUpdatePayload) error {
	touchesRegistry := req.Providers != nil || req.PrimaryModel != "" || req.FallbackModel != "" ||
		req.Communication != nil || req.Search != nil
	if !touchesRegistry {
		return nil
	}
	err := s.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
		applyRegistryProviders(reg, req.Providers)
		return applyRegistryFields(reg, req)
	})
	if err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}
	return nil
}

// applyRegistryProviders upserts each non-local provider's type and base URL.
func applyRegistryProviders(reg *models.RegistryData, providers map[string]models.ProviderItem) {
	if providers == nil {
		return
	}
	if reg.Providers == nil {
		reg.Providers = make(map[string]models.ProviderRegistryEntry)
	}
	for id, p := range providers {
		if id == "local" {
			continue // Infrastructure-only
		}
		entry := reg.Providers[id]
		entry.Type = p.Type
		entry.BaseURL = p.BaseURL
		reg.Providers[id] = entry
	}
}

// applyRegistryFields sets the primary and fallback model (which must exist), communication and search.
func applyRegistryFields(reg *models.RegistryData, req models.SystemUpdatePayload) error {
	if req.PrimaryModel != "" {
		if !models.ModelExists(reg, req.PrimaryModel) {
			return &models.ModelNotFoundError{Role: "primary", ModelName: req.PrimaryModel}
		}
		reg.PrimaryModel = req.PrimaryModel
	}
	if req.FallbackModel != "" {
		if !models.ModelExists(reg, req.FallbackModel) {
			return &models.ModelNotFoundError{Role: "fallback", ModelName: req.FallbackModel}
		}
		reg.FallbackModel = req.FallbackModel
	}
	if req.Communication != nil {
		reg.Communication = *req.Communication
	}
	if req.Search != nil {
		reg.Search = *req.Search
	}
	return nil
}

// applyServiceCredentials exports the service client id/secret to the process and the .env file.
func applyServiceCredentials(req models.SystemUpdatePayload) {
	envUpdates := map[string]string{}
	if req.ServiceClientID != "" {
		os.Setenv("SERVICE_CLIENT_ID", req.ServiceClientID)
		envUpdates["SERVICE_CLIENT_ID"] = req.ServiceClientID
	}
	if req.ServiceClientSecret != "" {
		os.Setenv("SERVICE_CLIENT_SECRET", req.ServiceClientSecret)
		envUpdates["SERVICE_CLIENT_SECRET"] = req.ServiceClientSecret
	}
	if len(envUpdates) > 0 {
		envPath, _ := env.EnvFilePaths()
		_ = env.UpdateEnvFile(envPath, envUpdates)
	}
}

// SystemUpdateChangesWhatRuns reports whether a system update changes a program the server runs or what it is started with:
// the environment and arguments of the model server, the model server's own path, or the GPU-stats program. That is
// command execution as the service user, so the admin API asks for the passphrase again before it. A value equal to the
// current one is not a change: the settings page sends back what it read, and an ordinary save must not ask.
func (s *AppContext) SystemUpdateChangesWhatRuns(req models.SystemUpdatePayload) bool {
	sys, set := s.GetSystem(), s.GetSettings()
	if req.Environment != nil && !equalEnvironment(req.Environment, sys.Server.Environment) {
		return true
	}
	if req.DefaultArgs != nil && !slices.Equal(req.DefaultArgs, set.Local.DefaultArgs) {
		return true
	}
	if req.GPUBinary != "" && req.GPUBinary != sys.Metrics.GPU.Binary {
		return true
	}
	local, ok := req.Providers["local"]
	if !ok {
		return false
	}
	return local.LlamaServerBinary != set.Local.LlamaServerBinary || (local.DefaultArgs != nil && !slices.Equal(local.DefaultArgs, set.Local.DefaultArgs))
}

// equalEnvironment compares two environments, an absent one being an empty one.
func equalEnvironment(a, b map[string]string) bool {
	return len(a) == len(b) && maps.Equal(a, b) || len(a) == 0 && len(b) == 0
}
