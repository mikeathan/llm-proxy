package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// attachDynamicTools gives the run path the tools the hosted Agents stack offers (ADR-0069). A host that could not be built leaves the run path with
// none, as it was before the stack could offer any.
func (s *AppServices) attachDynamicTools(h *stackHost) {
	if s.dynamicTools == nil || h == nil {
		return
	}
	if src, ok := h.stack.(stacksdk.ToolSource); ok {
		s.dynamicTools.Set(src)
	}
}

// legacyMCPServer is an entry of the `mcp_servers` list the registry kept before the Agents stack took MCP over (ADR-0069).
type legacyMCPServer struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	TLSCACert string `json:"tls_ca_cert"`
}

// The shape of a server name the stack accepts, and the most characters one has.
const (
	mcpNameMax    = 24
	mcpNameFiller = "s"
)

var mcpNameJunk = regexp.MustCompile(`[^a-z0-9]+`)

// migrateMCPServers copies the servers the registry listed into the Agents stack's settings, once: each keeps its address, its switch and its CA file,
// gets a name the stack accepts, and asks first (the new default). Then the registry is written again without the old list, so nothing reads it
// afterwards and a second start finds nothing to do. A name that had to change is logged. An entry whose address is not usable is still copied, so
// the Tools page can show it and the owner can fix or remove it.
func migrateMCPServers(dataMgr *storage.DataManager, agents agentsPolicy) error {
	raw, err := os.ReadFile(dataMgr.Paths().RegistryFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read the registry for the MCP servers: %w", err)
	}
	var old struct {
		MCPServers []legacyMCPServer `json:"mcp_servers"`
	}
	if err := json.Unmarshal(raw, &old); err != nil || len(old.MCPServers) == 0 {
		return nil
	}
	if len(agents.Get().MCPServers) == 0 {
		moved := convertLegacyMCP(old.MCPServers)
		if err := agents.Update(func(c *models.AgentsConfig) error {
			c.MCPServers = moved
			return nil
		}); err != nil {
			return fmt.Errorf("failed to copy the MCP servers into the Agents settings: %w", err)
		}
		logging.Info("the MCP servers of the registry were copied into the Agents stack", "count", len(moved))
	}
	// The registry store writes only the fields it knows, so an update that changes nothing drops the old list from the file.
	if err := dataMgr.Registry().Update(func(*models.RegistryData) error { return nil }); err != nil {
		return fmt.Errorf("failed to rewrite the registry without the old MCP servers: %w", err)
	}
	return nil
}

// convertLegacyMCP makes the old entries into the stack's: a name of lower-case letters, digits and hyphens that starts with a letter, at most
// mcpNameMax characters, and unique.
func convertLegacyMCP(old []legacyMCPServer) []models.MCPServerSetting {
	taken := map[string]bool{}
	out := make([]models.MCPServerSetting, 0, len(old))
	for _, o := range old {
		name := legacyMCPName(o.Name, taken)
		if name != o.Name {
			logging.Info("an MCP server was renamed to fit the Agents stack", "was", o.Name, "now", name)
		}
		taken[name] = true
		out = append(out, models.MCPServerSetting{Name: name, URL: o.URL, Enabled: o.Enabled, Trust: "ask", CACert: o.TLSCACert})
	}
	return out
}

func legacyMCPName(old string, taken map[string]bool) string {
	name := strings.Trim(mcpNameJunk.ReplaceAllString(strings.ToLower(old), "-"), "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = mcpNameFiller + "-" + name
	}
	name = strings.TrimRight(name[:min(len(name), mcpNameMax)], "-")
	candidate := name
	for n := 2; taken[candidate]; n++ {
		suffix := fmt.Sprintf("-%d", n)
		candidate = strings.TrimRight(name[:min(len(name), mcpNameMax-len(suffix))], "-") + suffix
	}
	return candidate
}
