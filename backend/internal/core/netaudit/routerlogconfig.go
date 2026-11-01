package netaudit

import (
	"fmt"

	"llm-proxy/models"
)

// RouterLogConfig reads and changes whether the program listens for the router's log.
type RouterLogConfig struct{ store PolicyStore }

func NewRouterLogConfig(store PolicyStore) *RouterLogConfig { return &RouterLogConfig{store: store} }

// Enabled reports whether the listener is switched on.
func (c *RouterLogConfig) Enabled() bool { return c.store.Get().RouterLogEnabled }

// Set switches the listener on or off.
func (c *RouterLogConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.RouterLogEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the router log setting: %w", err)
	}
	return nil
}
