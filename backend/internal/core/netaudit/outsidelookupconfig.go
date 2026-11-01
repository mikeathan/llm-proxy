package netaudit

import (
	"fmt"

	outside "llm-proxy/internal/core/netaudit/outsidelookup"
	"llm-proxy/models"
)

// OutsideLookupConfig reads and changes whether the audit asks an outside scan database what the internet can reach, and which
// service it asks.
type OutsideLookupConfig struct{ store PolicyStore }

func NewOutsideLookupConfig(store PolicyStore) *OutsideLookupConfig {
	return &OutsideLookupConfig{store: store}
}

// Enabled reports whether the lookup is switched on.
func (c *OutsideLookupConfig) Enabled() bool { return c.store.Get().OutsideLookupEnabled }

// URL is the service that is asked: the one set, or the default.
func (c *OutsideLookupConfig) URL() string {
	if u := c.store.Get().OutsideLookupURL; u != "" {
		return u
	}
	return outside.DefaultURL
}

// DefaultURL is the service used when none is set.
func (c *OutsideLookupConfig) DefaultURL() string { return outside.DefaultURL }

// Set switches the lookup on or off.
func (c *OutsideLookupConfig) Set(enabled bool) error {
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.OutsideLookupEnabled = enabled
		return nil
	})
}

// SetURL keeps the address of the service to ask, after checking it is an https address. An empty one goes back to the default.
func (c *OutsideLookupConfig) SetURL(raw string) error {
	kept := ""
	if raw != "" {
		var err error
		if kept, err = outside.ValidateBaseURL(raw); err != nil {
			return err
		}
		if kept == outside.DefaultURL {
			kept = ""
		}
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.OutsideLookupURL = kept
		return nil
	})
}

func (c *OutsideLookupConfig) update(fn func(*models.NetauditConfig) error) error {
	if err := c.store.Update(fn); err != nil {
		return fmt.Errorf("failed to save the outside lookup setting: %w", err)
	}
	return nil
}
