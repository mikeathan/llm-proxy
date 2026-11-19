package models

// AgentsConfig is the host-wide policy of the Agents stack (settings.yml → agents:). Like NetauditConfig it is not part of
// UserSettings, so saving the ordinary settings form cannot overwrite it. The parts have always run, so each switch is a negative
// flag: an install with nothing stored keeps them on.
type AgentsConfig struct {
	// ActivityOff stops the agent security record: refused tool calls, approvals and new destinations are no longer kept or
	// alerted about.
	ActivityOff bool `yaml:"activity_off,omitempty" json:"activity_off"`
	// SecretsPassOff stops the nightly look through workspace files for credentials.
	SecretsPassOff bool `yaml:"secrets_pass_off,omitempty" json:"secrets_pass_off"`
	// MCPOff switches the Tools layer off: no MCP server is dialled and none of their tools is offered.
	MCPOff bool `yaml:"mcp_off,omitempty" json:"mcp_off"`
	// MCPServers are the MCP servers the owner listed (ADR-0069). The stack reads and writes them through its Config port, so this shape is the
	// stack's own: a field the stack adds must be added here too, or a save would drop it (a test round-trips the two).
	MCPServers []MCPServerSetting `yaml:"mcp_servers,omitempty" json:"mcp_servers"`
}

// MCPServerSetting is one MCP server as the Agents stack keeps it. The sign-in value is sealed by the stack's vault and never stored in the clear.
type MCPServerSetting struct {
	Name          string         `yaml:"name" json:"name"`
	URL           string         `yaml:"url" json:"url"`
	Enabled       bool           `yaml:"enabled" json:"enabled"`
	Trust         string         `yaml:"trust,omitempty" json:"trust,omitempty"`
	Auth          MCPAuthSetting `yaml:"auth,omitempty" json:"auth,omitzero"`
	CACert        string         `yaml:"ca_cert,omitempty" json:"ca_cert,omitempty"`
	DisabledTools []string       `yaml:"disabled_tools,omitempty" json:"disabled_tools,omitempty"`
}

// MCPAuthSetting is how a server is signed in to: none, a bearer token, or one named header. Sealed is the value, encrypted.
type MCPAuthSetting struct {
	Kind   string `yaml:"kind,omitempty" json:"kind"`
	Header string `yaml:"header,omitempty" json:"header,omitempty"`
	Sealed string `yaml:"sealed,omitempty" json:"sealed,omitempty"`
}
