package models

type HostSettings struct {
	Sandboxing HostSandboxingConfig `json:"sandboxing"`
}

type HostSandboxingConfig struct {
	Enabled      bool   `json:"enabled"`
	MaxStorageGB int    `json:"max_storage_gb"`
	MaxMemoryMB  int    `json:"max_memory_mb"`
	Functional   bool   `json:"functional"`
}

func DefaultHostSettings() HostSettings {
	return HostSettings{
		Sandboxing: HostSandboxingConfig{
			Enabled:      true,
			MaxStorageGB: 2,
			MaxMemoryMB:  256,
		},
	}
}
