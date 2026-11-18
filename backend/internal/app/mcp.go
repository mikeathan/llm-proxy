package app

import (
	"context"
	"net"

	"llm-proxy/internal/core/mcp"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// configureMCP wires the registry change subscription into the MCP
// orchestrator and returns the node-herder facade used by the agent stack.
func configureMCP(dataMgr *storage.DataManager, logger logging.Logger, dialer func(context.Context, string, string) (net.Conn, error)) (nodeherder.MCPService, error) {
	// Initialize MCP Orchestrator
	orch := mcp.NewOrchestrator(logger)
	orch.DialContext = dialer

	// Subscribe Registry Updates -> MCP Orchestrator
	dataMgr.Registry().OnChange(func(reg models.RegistryData) {
		sys := dataMgr.System().Get()
		orch.Reload(context.Background(), translateMCPServers(reg.MCPServers), sys.Server.Bind)
	})

	// Initial Load
	currentReg := dataMgr.Registry().Get()
	sys := dataMgr.System().Get()
	orch.Reload(context.Background(), translateMCPServers(currentReg.MCPServers), sys.Server.Bind)

	return mcp.NewMCPNodeHerder(orch), nil
}

// translateMCPServers converts registry servers to internal model configs.
func translateMCPServers(reg []models.MCPServerRegistryEntry) []models.MCPServerConfig {
	out := make([]models.MCPServerConfig, len(reg))
	for i, s := range reg {
		out[i] = models.MCPServerConfig{
			Name:      s.Name,
			URL:       s.URL,
			Enabled:   s.Enabled,
			TLSCACert: s.TLSCACert,
		}
	}
	return out
}
