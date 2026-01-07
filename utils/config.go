package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"llm-proxy/models"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func LoadEnv() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("failed to load .env: %v", err)
	}

	envFile := ".env." + env
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("failed to load %s: %v", envFile, err)
	}
}

func Require(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("%s not set", key)
	}
	return val
}

func LoadServiceCredentials() (string, string, error) {
	clientID := os.Getenv("SERVICE_CLIENT_ID")
	clientSecret := os.Getenv("SERVICE_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("service credentials not configured")
	}

	return clientID, clientSecret, nil
}

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

func UpdateEnvFile(path string, updates map[string]string) error {
	current := map[string]string{}
	if existing, err := godotenv.Read(path); err == nil {
		current = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for key, value := range updates {
		current[key] = value
	}

	return godotenv.Write(current, path)
}
