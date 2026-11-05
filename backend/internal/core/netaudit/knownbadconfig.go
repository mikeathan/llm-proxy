package netaudit

import (
	"fmt"

	"llm-proxy/models"
)

// KnownBadConfig reads and changes whether the names devices ask for are compared with the malware list. Off until switched on.
type KnownBadConfig struct{ store PolicyStore }

func NewKnownBadConfig(store PolicyStore) *KnownBadConfig { return &KnownBadConfig{store: store} }

// Enabled reports whether the comparison is on.
func (c *KnownBadConfig) Enabled() bool { return c.store.Get().KnownBadEnabled }

// Set switches the comparison on or off.
func (c *KnownBadConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.KnownBadEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the known-bad names setting: %w", err)
	}
	return nil
}
