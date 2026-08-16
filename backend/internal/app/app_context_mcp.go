package app

import (
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func (s *AppContext) ListMCPServers() []models.MCPServerConfig {
	reg := s.dataMgr.Registry().Get()
	out := make([]models.MCPServerConfig, len(reg.MCPServers))
	for i, s := range reg.MCPServers {
		out[i] = models.MCPServerConfig{
			Name:    s.Name,
			URL:     s.URL,
			Enabled: s.Enabled,
		}
	}
	return out
}

func (s *AppContext) AddMCPServer(cfg models.MCPServerConfig) error {
	logging.Info("Adding new MCP server to registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		for _, existing := range c.MCPServers {
			if existing.Name == cfg.Name {
				return nil
			}
		}
		c.MCPServers = append(c.MCPServers, models.MCPServerRegistryEntry{
			Name:    cfg.Name,
			URL:     cfg.URL,
			Enabled: cfg.Enabled,
		})
		return nil
	})
}

func (s *AppContext) UpdateMCPServer(cfg models.MCPServerConfig) error {
	logging.Info("Updating MCP server in registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		for i, m := range c.MCPServers {
			if m.Name == cfg.Name {
				c.MCPServers[i] = models.MCPServerRegistryEntry{
					Name:    cfg.Name,
					URL:     cfg.URL,
					Enabled: cfg.Enabled,
				}
				return nil
			}
		}
		return nil
	})
}

func (s *AppContext) RemoveMCPServer(name string) error {
	logging.Info("Removing MCP server from registry", "name", name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) error {
		out := c.MCPServers[:0]
		for _, m := range c.MCPServers {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.MCPServers = out
		return nil
	})
}
