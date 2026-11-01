package netaudit

import (
	"slices"
	"testing"
	"time"
)

func TestDNSCheckConfig_IsOffAndUsesTheDefaultsUntilSet(t *testing.T) {
	c := NewDNSCheckConfig(&memPolicy{})
	v := c.View()
	if v.Enabled || v.IntervalMinutes != DefaultDNSCheckInterval || !slices.Equal(v.Names, v.DefaultNames) || v.ResolverURL != v.DefaultResolverURL {
		t.Errorf("defaults: %+v", v)
	}
	if len(v.Names) == 0 || v.DefaultResolverURL == "" || !slices.Equal(v.AllowedIntervals, AllowedDNSCheckIntervals) {
		t.Errorf("view: %+v", v)
	}
	s := c.Settings()
	if s.Enabled || s.Every != time.Duration(DefaultDNSCheckInterval)*time.Minute || !slices.Equal(s.Names, v.DefaultNames) {
		t.Errorf("settings: %+v", s)
	}
	if err := c.SetEnabled(true); err != nil || !c.View().Enabled || !c.Settings().Enabled {
		t.Fatalf("SetEnabled(true): %v", err)
	}
}

func TestDNSCheckConfig_AnIntervalOutsideTheChoicesChangesNothing(t *testing.T) {
	store := &memPolicy{}
	c := NewDNSCheckConfig(store)
	if err := c.SetInterval(30); err != nil || c.View().IntervalMinutes != 30 || c.Settings().Every != 30*time.Minute {
		t.Fatalf("30: %v %+v", err, c.View())
	}
	if err := c.SetInterval(1); err == nil || c.View().IntervalMinutes != 30 {
		t.Errorf("1 minute must be refused and change nothing: %v %+v", err, c.View())
	}
	if err := c.SetInterval(DefaultDNSCheckInterval); err != nil || store.cfg.DNSCheckIntervalMinutes != 0 {
		t.Errorf("the default is stored as nothing: %d %v", store.cfg.DNSCheckIntervalMinutes, err)
	}
}

func TestDNSCheckConfig_NamesAreCheckedBeforeTheyAreKept(t *testing.T) {
	store := &memPolicy{}
	c := NewDNSCheckConfig(store)
	if err := c.SetNames([]string{" Example.COM ", "example.com", "iana.org."}); err != nil || !slices.Equal(c.View().Names, []string{"example.com", "iana.org"}) {
		t.Fatalf("names: %v %v", c.View().Names, err)
	}
	if err := c.SetNames([]string{"not a name"}); err == nil || !slices.Equal(c.View().Names, []string{"example.com", "iana.org"}) {
		t.Errorf("a bad name must be refused and change nothing: %v %v", err, c.View().Names)
	}
	if err := c.SetNames(nil); err != nil || len(store.cfg.DNSCheckNames) != 0 || !slices.Equal(c.View().Names, c.View().DefaultNames) {
		t.Errorf("no names goes back to the defaults: %v %v", store.cfg.DNSCheckNames, err)
	}
	if err := c.SetNames(append([]string(nil), c.View().DefaultNames...)); err != nil || len(store.cfg.DNSCheckNames) != 0 {
		t.Errorf("the defaults are stored as nothing: %v %v", store.cfg.DNSCheckNames, err)
	}
}

func TestDNSCheckConfig_TheResolverAddressIsCheckedBeforeItIsKept(t *testing.T) {
	store := &memPolicy{}
	c := NewDNSCheckConfig(store)
	if err := c.SetResolverURL(" https://dns.example.org/q/ "); err != nil || c.View().ResolverURL != "https://dns.example.org/q" {
		t.Fatalf("url = %q, %v", c.View().ResolverURL, err)
	}
	for _, bad := range []string{"http://dns.example.org/q", "dns.example.org", "https://u:p@dns.example.org/q", "https://dns.example.org/q?x=1"} {
		if err := c.SetResolverURL(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if c.View().ResolverURL != "https://dns.example.org/q" {
		t.Errorf("a refused address must not replace the kept one: %q", c.View().ResolverURL)
	}
	if err := c.SetResolverURL(""); err != nil || store.cfg.DNSCheckResolverURL != "" || c.View().ResolverURL != c.View().DefaultResolverURL {
		t.Errorf("empty goes back to the default: %q %v", store.cfg.DNSCheckResolverURL, err)
	}
	if err := c.SetResolverURL(c.View().DefaultResolverURL); err != nil || store.cfg.DNSCheckResolverURL != "" {
		t.Errorf("the default address is stored as nothing: %q %v", store.cfg.DNSCheckResolverURL, err)
	}
}
