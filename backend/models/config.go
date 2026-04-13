package models

type Config struct {
	Server        ServerConfig            `json:"server"`
	Providers     map[string]ProviderItem `json:"providers"`
	Models        []ModelConfig           `json:"models"`
	Agents        []AgentDefinition       `json:"agents,omitempty"`
	WorkspacesDir string                  `json:"workspaces_dir,omitempty"`
	Metrics       MetricsConfig           `json:"metrics,omitempty"`
	MCPServers    []MCPServerConfig       `json:"mcp_servers,omitempty"`
	Guardrails    AgentGuardrailsConfig   `json:"guardrails,omitempty"`
	Communication CommunicationConfig     `json:"communication,omitempty"`
	Search        SearchConfig            `json:"search,omitempty"`
}

type AgentGuardrailsConfig struct {
	Global        GlobalGuardrailsConfig        `json:"global"`
	Terminal      TerminalGuardrailsConfig      `json:"terminal"`
	Search        SearchGuardrailsConfig        `json:"search"`
	Communication CommunicationGuardrailsConfig `json:"communication"`
	FileSystem    FileSystemGuardrailsConfig    `json:"filesystem"`
}

type GlobalGuardrailsConfig struct {
	BlockSecrets bool     `json:"block_secrets"`
	UserBlocked  []string `json:"user_blocked_patterns"`
}

type SearchGuardrailsConfig struct {
	Enabled      bool     `json:"enabled"`
	MaxQueryLen  int      `json:"max_query_len"`
	BlockedSites []string `json:"blocked_sites"`
}

type CommunicationGuardrailsConfig struct {
	Enabled       bool `json:"enabled"`
	RequireReview bool `json:"require_review"`
	MaxMessages   int  `json:"max_messages_per_task"`
}

type FileSystemGuardrailsConfig struct {
	Enabled           bool     `json:"enabled"`
	AllowedPaths      []string `json:"allowed_paths"`
	ReadOnly          bool     `json:"read_only"`
	MaxFileSizeKB     int      `json:"max_file_size_kb"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
	BlockedFilenames  []string `json:"blocked_filenames,omitempty"`
}

type TerminalGuardrailsConfig struct {
	Enabled         bool     `json:"enabled"`
	AllowedCommands []string `json:"allowed_commands"`
	BlockedPatterns []string `json:"blocked_patterns,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	MaxOutputSize   int      `json:"max_output_size_chars"`
}

type CommunicationConfig struct {
	Telegram struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"`
		ChatID  string `json:"chat_id"`
	} `json:"telegram"`
}

type SearchConfig struct {
	TavilyKey string `json:"tavily_key"`
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
	Type              string            `json:"type"`
	APIKeys           []APIKeyItem      `json:"api_keys,omitempty"`
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
	PrimaryModel      string            `json:"primary_model,omitempty"`
	FallbackModel     string            `json:"fallback_model,omitempty"`
}

type ModelConfig struct {
	Name           string            `json:"name"`
	Provider       string            `json:"provider"`
	Filename       string            `json:"filename,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Port           int               `json:"port,omitempty"`
	Path           string            `json:"-"`
	Environment    map[string]string `json:"environment,omitempty"`
	ProviderConfig ProviderConfig    `json:"provider_config,omitempty"`
}

type ProviderConfig struct {
	APIKey     string `json:"api_key,omitempty"`
	APIKeyName string `json:"api_key_name,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Region     string `json:"region,omitempty"`
}

type MetricsConfig struct {
	GPU GPUConfig `json:"gpu"`
}

type GPUConfig struct {
	Provider  string `json:"provider,omitempty"`
	Binary    string `json:"binary,omitempty"`
	Index     int    `json:"index,omitempty"`
	SysfsPath string `json:"sysfs_path,omitempty"`
}
