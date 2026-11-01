package netaudit

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

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

// Address is the public address the operator entered, or empty.
func (c *OutsideLookupConfig) Address() string { return c.store.Get().OutsideLookupAddress }

// SetAddress keeps the network's public address, for when the router states none. Only a public IPv4 address is kept; an empty
// one clears it.
func (c *OutsideLookupConfig) SetAddress(raw string) error {
	kept := strings.TrimSpace(raw)
	if kept != "" {
		addr, err := netip.ParseAddr(kept)
		if err != nil || outside.Classify(addr) != outside.Public {
			return errNotPublicAddress
		}
		kept = addr.String()
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.OutsideLookupAddress = kept
		return nil
	})
}

var errNotPublicAddress = errors.New("that is not a public IPv4 address: enter the address the internet sees for your network (not one that starts with 192.168, 10, 172.16 to 172.31, 100.64 to 100.127 or 169.254)")
