package netaudit

import (
	"fmt"

	"llm-proxy/models"
)

// NamesConfig reads and changes whether devices are asked their names. It is on unless switched off.
type NamesConfig struct{ store PolicyStore }

func NewNamesConfig(store PolicyStore) *NamesConfig { return &NamesConfig{store: store} }

// Enabled reports whether asking devices their names is on.
func (c *NamesConfig) Enabled() bool { return !c.store.Get().DeviceNamesOff }

// Set switches asking devices their names on or off.
func (c *NamesConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.DeviceNamesOff = !enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the device names setting: %w", err)
	}
	return nil
}
