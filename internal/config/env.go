package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadEnv loads environment variables from .env and .env.{APP_ENV} files.
func LoadEnv() {
	mainEnv, appEnv := EnvFilePaths()

	// Try loading .env
	_ = godotenv.Load(mainEnv)

	// Try loading .env.{env}
	_ = godotenv.Load(appEnv)
}

// GetAppEnv returns the current application environment (APP_ENV), defaulting to "development".
func GetAppEnv() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		return "development"
	}
	return env
}

// GetBindOverride returns the BIND environment variable if set.
func GetBindOverride() string {
	return os.Getenv("BIND")
}

// GetServiceCredentials returns the configured service client ID and secret.
// Returns empty strings if not configured.
func GetServiceCredentials() (string, string) {
	return os.Getenv("SERVICE_CLIENT_ID"), os.Getenv("SERVICE_CLIENT_SECRET")
}

// RequireServiceCredentials returns the service credentials or an error if they are missing.
func RequireServiceCredentials() (string, string, error) {
	id, secret := GetServiceCredentials()
	if id == "" || secret == "" {
		return "", "", fmt.Errorf("service credentials not configured")
	}
	return id, secret, nil
}

// EnvFilePaths returns the path to the main .env file and the environment-specific .env file.
// It tries to locate the file in the executable directory or current working directory.
func EnvFilePaths() (string, string) {
	baseDir := envBaseDir()
	env := GetAppEnv()

	mainParams := filepath.Join(baseDir, ".env")
	envParams := filepath.Join(baseDir, fmt.Sprintf(".env.%s", env))

	return mainParams, envParams
}

// UpdateEnvFile updates the specified .env file with the given key-value pairs.
func UpdateEnvFile(path string, updates map[string]string) error {
	// 1. Load existing
	current, err := godotenv.Read(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read env files: %w", err)
	}
	if current == nil {
		current = make(map[string]string)
	}

	// 2. Update
	for k, v := range updates {
		current[k] = v
	}

	// 3. Write back
	return godotenv.Write(current, path)
}

func envBaseDir() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		// Prepare for Symlinks
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
