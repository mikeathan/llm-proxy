package app

import (
	"fmt"
	"path/filepath"

	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// Tier 2: Registry
func (s *AppContext) GetRegistry() models.RegistryData {
	return s.dataMgr.Registry().Get()
}

func (s *AppContext) UpdateRegistry(fn func(*models.RegistryData)) error {
	return s.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
		fn(reg)
		return nil
	})
}

func (s *AppContext) Models() []models.ModelConfig {
	reg := s.dataMgr.Registry().Get()
	out := make([]models.ModelConfig, len(reg.Catalogue))
	for i, m := range reg.Catalogue {
		out[i] = models.ModelConfig{
			Name:     m.Name,
			Provider: m.ProviderID,
			Filename: m.ModelID,
			Port:     m.Port,
			Args:     m.Args,
			Prefill:  m.Prefill,
			ProviderConfig: &models.ProviderConfig{
				APIKeyName: m.CredentialID,
			},
		}
	}
	return out
}

func (s *AppContext) Providers() map[string]models.ProviderItem {
	reg := s.dataMgr.Registry().Get()
	settings := s.dataMgr.Settings().Get()
	out := make(map[string]models.ProviderItem)

	// 1. Populate from Registry
	for k, v := range reg.Providers {
		out[k] = models.ProviderItem{
			Type:    v.Type,
			BaseURL: v.BaseURL,
		}
	}

	// 2. Ensure 'local' is present and enriched from system config
	local := out["local"]
	if local.Type == "" {
		local.Type = "local"
	}
	local.LlamaServerBinary = settings.Local.LlamaServerBinary
	local.ModelDir = settings.Local.ModelDir
	local.DefaultArgs = settings.Local.DefaultArgs
	out["local"] = local

	return out
}

func (s *AppContext) GetGuardrails() models.AgentGuardrailsConfig {
	// Start with defaults from manifests
	cfg := tools.GetDefaultGuardrails()

	// Overlay with user overrides from settings.yml
	settings := s.dataMgr.Settings().Get()
	if settings.Guardrails != nil {
		cfg.MergeWith(settings.Guardrails)
	}
	return cfg
}

func (s *AppContext) PersistModel(cfg models.ModelConfig) error {
	logging.Info("Persisting new model to registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		for i, existing := range c.Catalogue {
			if existing.Name == cfg.Name {
				credID := ""
				if cfg.ProviderConfig != nil {
					credID = cfg.ProviderConfig.APIKeyName
				}
				c.Catalogue[i] = models.ModelRegistryEntry{
					ID:           cfg.Name,
					Name:         cfg.Name,
					ProviderID:   cfg.Provider,
					ModelID:      cfg.Filename,
					CredentialID: credID,
					Port:         cfg.Port,
					Args:         cfg.Args,
					Prefill:      cfg.Prefill,
					Metadata:     cfg.Metadata,
				}
				return nil
			}
		}
		credID := ""
		if cfg.ProviderConfig != nil {
			credID = cfg.ProviderConfig.APIKeyName
		}
		c.Catalogue = append(c.Catalogue, models.ModelRegistryEntry{
			ID:           cfg.Name,
			Name:         cfg.Name,
			ProviderID:   cfg.Provider,
			ModelID:      cfg.Filename,
			CredentialID: credID,
			Port:         cfg.Port,
			Args:         cfg.Args,
			Prefill:      cfg.Prefill,
			Metadata:     cfg.Metadata,
		})
		return nil
	})
}

func (s *AppContext) PersistReplaceModel(cfg models.ModelConfig) error {
	logging.Info("Replacing model in registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		replaced := false
		credID := ""
		if cfg.ProviderConfig != nil {
			credID = cfg.ProviderConfig.APIKeyName
		}
		newEntry := models.ModelRegistryEntry{
			ID:             cfg.Name,
			Name:           cfg.Name,
			ProviderID:     cfg.Provider,
			ModelID:        cfg.Filename,
			CredentialID:   credID,
			Port:           cfg.Port,
			Args:           cfg.Args,
			Prefill:        cfg.Prefill,
			TimeoutMinutes: cfg.TimeoutMinutes,
			Metadata:       cfg.Metadata,
		}
		for i, m := range c.Catalogue {
			if m.Name == cfg.Name {
				c.Catalogue[i] = newEntry
				replaced = true
				break
			}
		}
		if !replaced {
			c.Catalogue = append(c.Catalogue, newEntry)
		}
		return nil
	})
}

func (s *AppContext) PersistDeleteModel(name string) error {
	logging.Info("Deleting model from registry", "name", name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		out := c.Catalogue[:0]
		for _, m := range c.Catalogue {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.Catalogue = out
		// Clear any primary/fallback that pointed at the removed model.
		models.ClearDanglingModelRefs(c)
		return nil
	})
}

func (s *AppContext) ResolveModelPath(filename, explicitPath string) string {
	if explicitPath != "" && filepath.IsAbs(explicitPath) {
		return explicitPath
	}
	if filename == "" && explicitPath != "" {
		return explicitPath
	}
	if filepath.IsAbs(filename) {
		return filename
	}
	modelDir := s.GetSettings().Local.ModelDir
	if modelDir != "" {
		return filepath.Join(modelDir, filename)
	}
	if explicitPath != "" {
		return explicitPath
	}
	return filename
}

func (s *AppContext) DeleteProviderWithCleanup(provider string) error {
	if err := s.dataMgr.Secrets().DeleteAllProviderKeys(provider); err != nil {
		return fmt.Errorf("failed to delete keys for provider %q: %w", provider, err)
	}

	if err := s.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
		out := reg.Catalogue[:0]
		for _, m := range reg.Catalogue {
			if m.ProviderID != provider {
				out = append(out, m)
			}
		}
		reg.Catalogue = out
		// Remove the stale provider registry entry so a deleted provider does
		// not linger in the providers map (which also feeds the runtime's
		// cloud-provider count and the Settings provider list).
		delete(reg.Providers, provider)
		// Clear any primary/fallback that pointed at a now-removed model.
		models.ClearDanglingModelRefs(reg)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to cleanup models for provider %q: %w", provider, err)
	}

	return nil
}
