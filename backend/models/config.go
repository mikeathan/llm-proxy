package models

type Config struct {
	Server        ServerConfig            `json:"server"`
	Providers     map[string]ProviderItem `json:"providers"` // Refactored Providers map
	Models        []ModelConfig           `json:"models"`
	Agents        []AgentDefinition       `json:"agents,omitempty"` // New Agents Registry
	WorkspacesDir string                  `json:"workspaces_dir,omitempty"`
	Metrics       MetricsConfig           `json:"metrics,omitempty"`
	MCPServers    []MCPServerConfig       `json:"mcp_servers,omitempty"`
}

type AgentDefinition struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ProviderID   string   `json:"provider_id"`
	ModelID      string   `json:"model_id"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
}

type APIKeyItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ProviderItem struct {
	Type              string            `json:"type"` // local, openai, gemini, etc.
	APIKeys           []APIKeyItem      `json:"api_keys,omitempty"` // Support for multiple named API keys
	BaseURL           string            `json:"base_url,omitempty"`
	ProjectID         string            `json:"project_id,omitempty"`
	Region            string            `json:"region,omitempty"`
	LlamaServerBinary string            `json:"llama_server_binary,omitempty"`
	ModelDir          string            `json:"model_dir,omitempty"`
	DefaultArgs       []string          `json:"default_args,omitempty"`
	Environment       map[string]string `json:"environment,omitempty"`
}

type MCPServerConfig struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type ServerConfig struct {
	Bind              string            `json:"bind"`
	ModelHost         string            `json:"model_host"`
	IdleTimeoutSecs   int               `json:"idle_timeout_seconds"`
	LlamaServerBinary string            `json:"llama_server_binary"`
	DefaultArgs       []string          `json:"default_args"`
	Environment       map[string]string `json:"environment"`
	DefaultModel      string            `json:"default_model,omitempty"`
	PrimaryModel      string            `json:"primary_model,omitempty"`
	FallbackModel     string            `json:"fallback_model,omitempty"`
}

type ModelConfig struct {
	Name           string            `json:"name"`
	Provider       string            `json:"provider"` // local, openai, vertex, gemini, openrouter, nim
	Filename       string            `json:"filename,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Port           int               `json:"port,omitempty"`
	Path           string            `json:"-"` // resolved absolute path, not persisted
	Environment    map[string]string `json:"environment,omitempty"`
	ProviderConfig ProviderConfig    `json:"provider_config,omitempty"`
}

type ProviderConfig struct {
	APIKey     string `json:"api_key,omitempty"`
	APIKeyName string `json:"api_key_name,omitempty"` // Look up by name in ProviderItem.APIKeys
	BaseURL    string `json:"base_url,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Region     string `json:"region,omitempty"`
}

type MetricsConfig struct {
	GPU GPUConfig `json:"gpu"`
}

type GPUConfig struct {
	Provider  string `json:"provider,omitempty"`   // auto, nvidia-smi, rocm-smi, amdgpu_top, sysfs, none
	Binary    string `json:"binary,omitempty"`     // optional override path
	Index     int    `json:"index,omitempty"`      // GPU index to query (0-based)
	SysfsPath string `json:"sysfs_path,omitempty"` // optional override for sysfs device folder
}
