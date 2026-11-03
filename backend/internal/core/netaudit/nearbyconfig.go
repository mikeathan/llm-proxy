package netaudit

import (
	"fmt"

	"llm-proxy/models"
)

// NearbyConfig reads and changes whether Nearby discovery is on (Constitution I.9). It is off until the operator turns it on.
type NearbyConfig struct{ store PolicyStore }

func NewNearbyConfig(store PolicyStore) *NearbyConfig { return &NearbyConfig{store: store} }

// Enabled reports whether Nearby is switched on.
func (c *NearbyConfig) Enabled() bool { return c.store.Get().NearbyEnabled }

// Set switches Nearby on or off.
func (c *NearbyConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.NearbyEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the Nearby setting: %w", err)
	}
	return nil
}
