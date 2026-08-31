package app

import (
	"context"
	"fmt"
	"os"

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
	return settings
}

func (s *AppContext) UpdateHostSettings(settings models.HostSettings) error {
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
	// 1. Update Infrastructure (SystemConfig)
	err := s.dataMgr.System().Update(func(sys *models.SystemConfig) error {
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

		// 2. Sync GPU Configuration (moved into transaction for persistence)
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
		if req.DefaultArgs != nil {
			set.Local.DefaultArgs = req.DefaultArgs
		}
		if local, ok := req.Providers["local"]; ok {
			// We allow clearing these fields by removing the != "" check
			set.Local.LlamaServerBinary = local.LlamaServerBinary
			set.Local.ModelDir = local.ModelDir
			if local.DefaultArgs != nil {
				set.Local.DefaultArgs = local.DefaultArgs
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}

	// 2. Registry and Infrastructure updates (Sync handled by subscribers)

	// 3. Update Registry Providers
	if req.Providers != nil {
		err = s.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
			if reg.Providers == nil {
				reg.Providers = make(map[string]models.ProviderRegistryEntry)
			}
			for id, p := range req.Providers {
				if id == "local" {
					continue // Infrastructure-only
				}
				entry := reg.Providers[id]
				entry.Type = p.Type
				entry.BaseURL = p.BaseURL
				reg.Providers[id] = entry
			}
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
		})
		if err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	} else if req.PrimaryModel != "" || req.FallbackModel != "" || req.Communication != nil || req.Search != nil {
		// Just update registry items if providers weren't involved
		err = s.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
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
		})
		if err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	}

	// 5. Update Environment Variables (.env)
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
