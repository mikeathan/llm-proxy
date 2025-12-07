package models

type Config struct {
	Server ServerConfig  `json:"server"`
	Models []ModelConfig `json:"models"`
}

type ServerConfig struct {
	Bind              string   `json:"bind"`
	ModelHost         string   `json:"model_host"`
	IdleTimeoutSecs   int      `json:"idle_timeout_seconds"`
	LlamaServerBinary string   `json:"llama_server_binary"`
	DefaultArgs       []string `json:"default_args"`
}

type ModelConfig struct {
	Name string   `json:"name"`
	Path string   `json:"path"`
	Args []string `json:"args"`
	Port int      `json:"port"`
}
