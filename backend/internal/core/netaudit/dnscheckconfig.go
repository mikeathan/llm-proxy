package netaudit

import (
	"fmt"
	"slices"
	"time"

	"llm-proxy/internal/core/netaudit/dnscheck"
	"llm-proxy/models"
)

// AllowedDNSCheckIntervals are the minutes between DNS checks an operator may choose.
var AllowedDNSCheckIntervals = []int{15, 30, 60, 120, 360}

// DefaultDNSCheckInterval is the interval used until the operator chooses one.
const DefaultDNSCheckInterval = 60

// DNSCheckView is the DNS check's settings as the operator sees them.
type DNSCheckView struct {
	Enabled          bool     `json:"enabled"`
	IntervalMinutes  int      `json:"interval_minutes"`
	AllowedIntervals []int    `json:"allowed_intervals"`
	Names            []string `json:"names"`
	DefaultNames     []string `json:"default_names"`
	// Server is the router's DNS server; empty means the default gateway.
	Server             string `json:"server"`
	ResolverURL        string `json:"resolver_url"`
	DefaultResolverURL string `json:"default_resolver_url"`
}

// DNSCheckConfig reads and changes the DNS check's settings in the network-audit policy.
type DNSCheckConfig struct{ store PolicyStore }

func NewDNSCheckConfig(store PolicyStore) *DNSCheckConfig { return &DNSCheckConfig{store: store} }

func dnsInterval(minutes int) int {
	if minutes == 0 {
		return DefaultDNSCheckInterval
	}
	return minutes
}

func (c *DNSCheckConfig) names() []string {
	if n := c.store.Get().DNSCheckNames; len(n) > 0 {
		return slices.Clone(n)
	}
	return slices.Clone(dnscheck.DefaultNames)
}

// View returns the stored settings with the defaults filled in.
func (c *DNSCheckConfig) View() DNSCheckView {
	cfg := c.store.Get()
	url := cfg.DNSCheckResolverURL
	if url == "" {
		url = dnscheck.DefaultResolverURL
	}
	return DNSCheckView{
		Enabled:            cfg.DNSCheckEnabled,
		IntervalMinutes:    dnsInterval(cfg.DNSCheckIntervalMinutes),
		AllowedIntervals:   AllowedDNSCheckIntervals,
		Names:              c.names(),
		DefaultNames:       slices.Clone(dnscheck.DefaultNames),
		Server:             cfg.DNSCheckServer,
		ResolverURL:        url,
		DefaultResolverURL: dnscheck.DefaultResolverURL,
	}
}

// Settings is what the collector itself reads for each cycle.
func (c *DNSCheckConfig) Settings() DNSSettings {
	cfg := c.store.Get()
	return DNSSettings{
		Enabled: cfg.DNSCheckEnabled,
		Every:   time.Duration(dnsInterval(cfg.DNSCheckIntervalMinutes)) * time.Minute,
		Names:   c.names(),
	}
}

// SetEnabled switches the check on or off.
func (c *DNSCheckConfig) SetEnabled(enabled bool) error {
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckEnabled = enabled
		return nil
	})
}

// SetInterval chooses how often the check runs; 0 is the default. A refused interval changes nothing.
func (c *DNSCheckConfig) SetInterval(minutes int) error {
	minutes = dnsInterval(minutes)
	if !slices.Contains(AllowedDNSCheckIntervals, minutes) {
		return fmt.Errorf("%w: %d minutes", ErrBadPresenceInterval, minutes)
	}
	stored := minutes
	if minutes == DefaultDNSCheckInterval {
		stored = 0
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckIntervalMinutes = stored
		return nil
	})
}

// SetNames keeps the names to compare after checking them. No names, or exactly the defaults, goes back to the defaults.
func (c *DNSCheckConfig) SetNames(in []string) error {
	var kept []string
	if len(in) > 0 {
		var err error
		if kept, err = dnscheck.ValidateNames(in); err != nil {
			return err
		}
		if slices.Equal(kept, dnscheck.DefaultNames) {
			kept = nil
		}
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckNames = kept
		return nil
	})
}

// SetServer keeps the router's DNS server after checking it is a private IPv4 address. An empty one goes back to the gateway.
func (c *DNSCheckConfig) SetServer(raw string) error {
	kept, err := dnscheck.ValidateServer(raw)
	if err != nil {
		return err
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckServer = kept
		return nil
	})
}

// SetResolverURL keeps the outside resolver's address after checking it is an https address. An empty one goes back to the default.
func (c *DNSCheckConfig) SetResolverURL(raw string) error {
	kept := ""
	if raw != "" {
		var err error
		if kept, err = dnscheck.ValidateResolverURL(raw); err != nil {
			return err
		}
		if kept == dnscheck.DefaultResolverURL {
			kept = ""
		}
	}
	return c.update(func(cfg *models.NetauditConfig) error {
		cfg.DNSCheckResolverURL = kept
		return nil
	})
}

func (c *DNSCheckConfig) update(fn func(*models.NetauditConfig) error) error {
	if err := c.store.Update(fn); err != nil {
		return fmt.Errorf("failed to save the DNS check setting: %w", err)
	}
	return nil
}
