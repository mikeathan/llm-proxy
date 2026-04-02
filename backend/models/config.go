package models

type Config struct {
	Server        ServerConfig      `json:"server"`
	Models        []ModelConfig     `json:"models"`
	ModelDir      string            `json:"model_dir"`
	WorkspacesDir string            `json:"workspaces_dir,omitempty"`
	Metrics       MetricsConfig     `json:"metrics,omitempty"`
	MCPServers    []MCPServerConfig `json:"mcp_servers,omitempty"`
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
}

type ModelConfig struct {
	Name        string            `json:"name"`
	Filename    string            `json:"filename"`
	Args        []string          `json:"args"`
	Port        int               `json:"port"`
	Path        string            `json:"-"` // resolved absolute path, not persisted
	Environment map[string]string `json:"environment"`
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
