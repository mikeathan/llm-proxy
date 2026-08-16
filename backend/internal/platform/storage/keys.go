package storage

import (
	"llm-proxy/internal/platform/paths"
)

// GetMasterKey returns the effective AES-256 master key for the given config
// directory: the LLM_MASTER_KEY env var when set (and valid), otherwise the
// hex-encoded master.key file in configDir with its integrity hash verified.
// A missing or corrupt key is a loud error — it is never silently replaced by
// a nil key (C4). Callers are expected to have seeded the directory first.
func GetMasterKey(configDir string) ([]byte, error) {
	return (paths.Paths{ConfigDir: configDir}).LoadMasterKey()
}
