package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"llm-proxy/models"
)

type HostSettingsStore struct {
	configPath string
}

func NewHostSettingsStore() *HostSettingsStore {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to local process dir if home is totally busted
		return &HostSettingsStore{configPath: "host.json"} 
	}
	
	configDir := filepath.Join(home, ".config", "llm-proxy")
	_ = os.MkdirAll(configDir, 0700)
	
	return &HostSettingsStore{
		configPath: filepath.Join(configDir, "host.json"),
	}
}

func (s *HostSettingsStore) Read() (models.HostSettings, error) {
	config := models.DefaultHostSettings()
	
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Write the default settings if they don't exist yet
			_ = s.Write(config)
			return config, nil
		}
		return config, fmt.Errorf("failed to read host settings: %w", err)
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to decode host settings: %w", err)
	}
	
	return config, nil
}

func (s *HostSettingsStore) Write(cfg models.HostSettings) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Write with strict 0600 permissions
	return os.WriteFile(s.configPath, data, 0600)
}
