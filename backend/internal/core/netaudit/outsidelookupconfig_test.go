package netaudit

import "testing"

func TestOutsideLookupConfig_IsOffAndUsesTheDefaultServiceUntilSet(t *testing.T) {
	c := NewOutsideLookupConfig(&memPolicy{})
	if c.Enabled() {
		t.Fatal("the outside lookup must be off by default")
	}
	if c.URL() != "https://internetdb.shodan.io" || c.DefaultURL() != c.URL() {
		t.Errorf("url = %q default = %q", c.URL(), c.DefaultURL())
	}
	if err := c.Set(true); err != nil || !c.Enabled() {
		t.Fatalf("Set(true): %v", err)
	}
	if err := c.Set(false); err != nil || c.Enabled() {
		t.Fatalf("Set(false): %v", err)
	}
}

func TestOutsideLookupConfig_TheServiceAddressIsCheckedBeforeItIsKept(t *testing.T) {
	store := &memPolicy{}
	c := NewOutsideLookupConfig(store)
	if err := c.SetURL(" https://lookup.example.org/db/ "); err != nil || c.URL() != "https://lookup.example.org/db" {
		t.Fatalf("url = %q, %v", c.URL(), err)
	}
	for _, bad := range []string{"http://lookup.example.org", "lookup.example.org", "https://u:p@lookup.example.org", "https://x.example/?k=v"} {
		if err := c.SetURL(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if c.URL() != "https://lookup.example.org/db" {
		t.Errorf("a refused address must not replace the kept one: %q", c.URL())
	}
	if err := c.SetURL(""); err != nil || c.URL() != c.DefaultURL() || store.cfg.OutsideLookupURL != "" {
		t.Errorf("an empty address goes back to the default: %q, stored %q, %v", c.URL(), store.cfg.OutsideLookupURL, err)
	}
}
