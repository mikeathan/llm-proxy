package utils

import (
	"encoding/json"
	"llm-proxy/models"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func LoadConfig(path string) (*models.Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg models.Config
	if err := json.Unmarshal(f, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func SaveConfig(path string, cfg *models.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadEnvOverrides(cfg *models.Config) {
	godotenv.Load()

	if v := os.Getenv("BIND"); v != "" {
		cfg.Server.Bind = v
	}
	if v := os.Getenv("IDLE_TIMEOUT_SECS"); v != "" {
		cfg.Server.IdleTimeoutSecs, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("LLAMA_BINARY"); v != "" {
		cfg.Server.LlamaServerBinary = v
	}
}
