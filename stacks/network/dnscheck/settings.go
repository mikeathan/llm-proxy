package dnscheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// AllowedIntervals are the minutes between DNS checks an operator may choose.
var AllowedIntervals = []int{15, 30, 60, 120, 360}

// DefaultInterval is the interval used until the operator chooses one.
const DefaultInterval = 60

// ErrBadInterval means the interval is not one of AllowedIntervals.
var ErrBadInterval = errors.New("that interval is not one of the choices")

// The settings document's keys. The document is the core's `netaudit` one: this stack reads and writes only these six fields and
// names no others, so a save cannot touch the rest.
const (
	keyEnabled     = "dns_check_enabled"
	keyInterval    = "dns_check_interval_minutes"
	keyNames       = "dns_check_names"
	keyServer      = "dns_check_server"
	keyResolverURL = "dns_check_resolver_url"
	// keyFinding switches on the finding the check offers the nightly report (ADR-0036). It is off until the operator has seen the check catch
	// a resolver that lies.
	keyFinding = "dns_check_finding_enabled"
)

// document is this stack's five fields of the settings document; every other field is ignored on read.
type document struct {
	Enabled     bool     `json:"dns_check_enabled"`
	Interval    int      `json:"dns_check_interval_minutes"`
	Names       []string `json:"dns_check_names"`
	Server      string   `json:"dns_check_server"`
	ResolverURL string   `json:"dns_check_resolver_url"`
	Finding     bool     `json:"dns_check_finding_enabled"`
}

// View is the DNS check's settings as the operator sees them.
type View struct {
	Enabled          bool     `json:"enabled"`
	IntervalMinutes  int      `json:"interval_minutes"`
	AllowedIntervals []int    `json:"allowed_intervals"`
	Names            []string `json:"names"`
	DefaultNames     []string `json:"default_names"`
	// Server is the router's DNS server; empty means the default gateway.
	Server             string `json:"server"`
	ResolverURL        string `json:"resolver_url"`
	DefaultResolverURL string `json:"default_resolver_url"`
	// FindingEnabled is whether a persistent difference is offered to the nightly report as a finding.
	FindingEnabled bool `json:"finding_enabled"`
}

// Config reads and changes the DNS check's settings through the core's settings port.
type Config struct{ Doc stacksdk.Config }

func interval(minutes int) int {
	if minutes == 0 {
		return DefaultInterval
	}
	return minutes
}

func (c Config) load(ctx context.Context) (document, error) {
	raw, err := c.Doc.Load(ctx)
	if err != nil {
		return document{}, fmt.Errorf("failed to read the DNS check settings: %w", err)
	}
	var d document
	if err := json.Unmarshal(raw, &d); err != nil {
		return document{}, fmt.Errorf("failed to read the DNS check settings: %w", err)
	}
	return d, nil
}

func names(d document) []string {
	if len(d.Names) > 0 {
		return slices.Clone(d.Names)
	}
	return slices.Clone(DefaultNames)
}

// View returns the stored settings with the defaults filled in.
func (c Config) View(ctx context.Context) (View, error) {
	d, err := c.load(ctx)
	if err != nil {
		return View{}, err
	}
	url := d.ResolverURL
	if url == "" {
		url = DefaultResolverURL
	}
	return View{
		Enabled:            d.Enabled,
		IntervalMinutes:    interval(d.Interval),
		AllowedIntervals:   AllowedIntervals,
		Names:              names(d),
		DefaultNames:       slices.Clone(DefaultNames),
		Server:             d.Server,
		ResolverURL:        url,
		DefaultResolverURL: DefaultResolverURL,
		FindingEnabled:     d.Finding,
	}, nil
}

// Settings is what the collector itself reads for each cycle. Settings that cannot be read count as switched off: nothing is sent.
func (c Config) Settings(ctx context.Context) Settings {
	d, err := c.load(ctx)
	if err != nil {
		return Settings{}
	}
	return Settings{Enabled: d.Enabled, Every: time.Duration(interval(d.Interval)) * time.Minute, Names: names(d)}
}

// Server is the router's DNS server the operator set, or empty.
func (c Config) Server(ctx context.Context) string {
	d, _ := c.load(ctx)
	return d.Server
}

// ResolverURL is the outside resolver's address, with the default filled in.
func (c Config) ResolverURL(ctx context.Context) string {
	v, _ := c.View(ctx)
	return v.ResolverURL
}

// save sends only the named fields; the core merges them over the rest.
func (c Config) save(ctx context.Context, fields map[string]any) error {
	doc, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to save the DNS check setting: %w", err)
	}
	if err := c.Doc.Save(ctx, doc); err != nil {
		return fmt.Errorf("failed to save the DNS check setting: %w", err)
	}
	return nil
}

// SetEnabled switches the check on or off.
func (c Config) SetEnabled(ctx context.Context, enabled bool) error {
	return c.save(ctx, map[string]any{keyEnabled: enabled})
}

// SetFindingEnabled switches on or off the finding the check offers the nightly report.
func (c Config) SetFindingEnabled(ctx context.Context, enabled bool) error {
	return c.save(ctx, map[string]any{keyFinding: enabled})
}

// SetInterval chooses how often the check runs; 0 is the default. A refused interval changes nothing.
func (c Config) SetInterval(ctx context.Context, minutes int) error {
	minutes = interval(minutes)
	if !slices.Contains(AllowedIntervals, minutes) {
		return fmt.Errorf("%w: %d minutes", ErrBadInterval, minutes)
	}
	stored := minutes
	if minutes == DefaultInterval {
		stored = 0
	}
	return c.save(ctx, map[string]any{keyInterval: stored})
}

// SetNames keeps the names to compare after checking them. No names, or exactly the defaults, goes back to the defaults.
func (c Config) SetNames(ctx context.Context, in []string) error {
	var kept []string
	if len(in) > 0 {
		var err error
		if kept, err = ValidateNames(in); err != nil {
			return err
		}
		if slices.Equal(kept, DefaultNames) {
			kept = nil
		}
	}
	return c.save(ctx, map[string]any{keyNames: kept})
}

// SetServer keeps the router's DNS server after checking it is a private IPv4 address. An empty one goes back to the gateway.
func (c Config) SetServer(ctx context.Context, raw string) error {
	kept, err := ValidateServer(raw)
	if err != nil {
		return err
	}
	return c.save(ctx, map[string]any{keyServer: kept})
}

// SetResolverURL keeps the outside resolver's address after checking it is an https address. An empty one goes back to the default.
func (c Config) SetResolverURL(ctx context.Context, raw string) error {
	kept := ""
	if raw != "" {
		var err error
		if kept, err = ValidateResolverURL(raw); err != nil {
			return err
		}
		if kept == DefaultResolverURL {
			kept = ""
		}
	}
	return c.save(ctx, map[string]any{keyResolverURL: kept})
}
