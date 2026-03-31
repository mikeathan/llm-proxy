package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"llm-proxy/models"
)

// ConfigManager handles thread-safe configuration access and atomic updates.
type ConfigManager struct {
	mu         sync.RWMutex
	config     *models.Config
	configPath string
	listeners  []func(models.Config)
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// OnChange registers a callback to be called when configuration updates.
func (chk *ConfigManager) OnChange(listener func(models.Config)) {
	chk.mu.Lock()
	defer chk.mu.Unlock()
	chk.listeners = append(chk.listeners, listener)
}

// Load reads the configuration from the file system and applies environment overrides.
func (chk *ConfigManager) Load() error {
	chk.mu.Lock()
	defer chk.mu.Unlock()

	// Load .env files first
	LoadEnv()

	// Load JSON config
	data, err := os.ReadFile(chk.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg models.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply overrides
	// applyEnvOverrides(&cfg) - Removed to enforce config.json as source of truth

	chk.config = &cfg
	return nil
}

// GetConfig returns a thread-safe copy of the current configuration.
func (chk *ConfigManager) GetConfig() models.Config {
	chk.mu.RLock()
	defer chk.mu.RUnlock()
	if chk.config == nil {
		return models.Config{}
	}
	// Return a copy by value
	return *chk.config
}

// UpdateMCPServers updates the MCP servers list and persists the configuration.
func (chk *ConfigManager) UpdateMCPServers(servers []models.MCPServerConfig) error {
	chk.mu.Lock()
	defer chk.mu.Unlock()

	if chk.config == nil {
		return fmt.Errorf("config not loaded")
	}

	// Update in-memory state
	chk.config.MCPServers = servers

	// Notify listeners
	cfgCopy := *chk.config
	for _, l := range chk.listeners {
		l(cfgCopy)
	}

	// Persist to disk atomically
	return chk.atomicSave()
}

// Update modifies the configuration using a callback and persists it.
// It is thread-safe and notifies listeners.
func (chk *ConfigManager) Update(fn func(*models.Config)) error {
	chk.mu.Lock()
	defer chk.mu.Unlock()

	if chk.config == nil {
		return fmt.Errorf("config not loaded")
	}

	// Apply modification
	fn(chk.config)

	// Notify listeners
	cfgCopy := *chk.config
	for _, l := range chk.listeners {
		l(cfgCopy)
	}

	// Persist
	return chk.atomicSave()
}

// atomicSave writes the current config to a temporary file and renames it.
// Caller must hold lock.
func (chk *ConfigManager) atomicSave() error {
	data, err := json.MarshalIndent(chk.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(chk.configPath)
	tmpFile, err := os.CreateTemp(dir, "config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName) // Cleanup on error

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write config data: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpName, chk.configPath); err != nil {
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	return nil
}

// Helper functions
