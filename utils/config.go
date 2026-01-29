package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"llm-proxy/models"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const DefaultConfigPath = "config/config.json"

func LoadEnv() {
	envPath, envFile := EnvFilePaths()

	if err := godotenv.Load(envPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("failed to load %s: %v", envPath, err)
	}

	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("failed to load %s: %v", envFile, err)
	}
}

func EnvFilePaths() (string, string) {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	baseDir := envBaseDir()
	return filepath.Join(baseDir, ".env"), filepath.Join(baseDir, ".env."+env)
}

func GetAbsoluteConfigPath(configPath string) string {
	if configPath == "" {
		return ""
	}
	if absPath, err := filepath.Abs(configPath); err == nil {
		return absPath
	}
	return configPath
}

func envBaseDir() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
			return filepath.Dir(resolved)
		}
		return filepath.Dir(exe)
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}
	return "."
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

func GetMCPServerURL() (string, error) {
	mcpURL := os.Getenv("MCP_SERVER_SSE_URL")
	if mcpURL == "" {
		return "", fmt.Errorf("MCP_SERVER_SSE_URL not set")
	}
	return mcpURL, nil
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

	return writeEnvFile(path, current)
}

func writeEnvFile(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, values[key]))
	}
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	return os.WriteFile(path, []byte(data), 0644)
}
