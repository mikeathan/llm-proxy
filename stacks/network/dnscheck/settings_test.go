package dnscheck

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

var ctx = context.Background()

// memDoc is the core's settings document as the port shows it: JSON in, JSON out, and a save that merges what it names over the rest.
type memDoc struct{ fields map[string]json.RawMessage }

func (m *memDoc) Load(context.Context) (json.RawMessage, error) {
	return json.Marshal(m.fields)
}

func (m *memDoc) Save(_ context.Context, doc json.RawMessage) error {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(doc, &in); err != nil {
		return err
	}
	if m.fields == nil {
		m.fields = map[string]json.RawMessage{}
	}
	for k, v := range in {
		m.fields[k] = v
	}
	return nil
}

func (m *memDoc) names() []string {
	var out []string
	_ = json.Unmarshal(m.fields["dns_check_names"], &out)
	return out
}

func (m *memDoc) str(key string) string {
	var out string
	_ = json.Unmarshal(m.fields[key], &out)
	return out
}

func (m *memDoc) num(key string) int {
	var out int
	_ = json.Unmarshal(m.fields[key], &out)
	return out
}

func view(c Config) View {
	v, err := c.View(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

func TestConfig_IsOffAndUsesTheDefaultsUntilSet(t *testing.T) {
	c := Config{Doc: &memDoc{}}
	v := view(c)
	usesDefaults := slices.Equal(v.Names, v.DefaultNames) && v.ResolverURL == v.DefaultResolverURL
	if v.Enabled || v.IntervalMinutes != DefaultInterval || !usesDefaults {
		t.Errorf("defaults: %+v", v)
	}
	if len(v.Names) == 0 || v.DefaultResolverURL == "" || !slices.Equal(v.AllowedIntervals, AllowedIntervals) {
		t.Errorf("view: %+v", v)
	}
	s := c.Settings(ctx)
	if s.Enabled || s.Every != time.Duration(DefaultInterval)*time.Minute || !slices.Equal(s.Names, v.DefaultNames) {
		t.Errorf("settings: %+v", s)
	}
	if err := c.SetEnabled(ctx, true); err != nil || !view(c).Enabled || !c.Settings(ctx).Enabled {
		t.Fatalf("SetEnabled(true): %v", err)
	}
}

func TestConfig_AnIntervalOutsideTheChoicesChangesNothing(t *testing.T) {
	store := &memDoc{}
	c := Config{Doc: store}
	if err := c.SetInterval(ctx, 30); err != nil || view(c).IntervalMinutes != 30 || c.Settings(ctx).Every != 30*time.Minute {
		t.Fatalf("30: %v %+v", err, view(c))
	}
	if err := c.SetInterval(ctx, 1); err == nil || view(c).IntervalMinutes != 30 {
		t.Errorf("1 minute must be refused and change nothing: %v %+v", err, view(c))
	}
	if err := c.SetInterval(ctx, DefaultInterval); err != nil || store.num("dns_check_interval_minutes") != 0 {
		t.Errorf("the default is stored as nothing: %d %v", store.num("dns_check_interval_minutes"), err)
	}
}

func TestConfig_NamesAreCheckedBeforeTheyAreKept(t *testing.T) {
	store := &memDoc{}
	c := Config{Doc: store}
	err := c.SetNames(ctx, []string{" Example.COM ", "example.com", "iana.org."})
	if err != nil || !slices.Equal(view(c).Names, []string{"example.com", "iana.org"}) {
		t.Fatalf("names: %v %v", view(c).Names, err)
	}
	if err := c.SetNames(ctx, []string{"not a name"}); err == nil || !slices.Equal(view(c).Names, []string{"example.com", "iana.org"}) {
		t.Errorf("a bad name must be refused and change nothing: %v %v", err, view(c).Names)
	}
	if err := c.SetNames(ctx, nil); err != nil || len(store.names()) != 0 || !slices.Equal(view(c).Names, view(c).DefaultNames) {
		t.Errorf("no names goes back to the defaults: %v %v", store.names(), err)
	}
	if err := c.SetNames(ctx, append([]string(nil), view(c).DefaultNames...)); err != nil || len(store.names()) != 0 {
		t.Errorf("the defaults are stored as nothing: %v %v", store.names(), err)
	}
}

func TestConfig_TheResolverAddressIsCheckedBeforeItIsKept(t *testing.T) {
	store := &memDoc{}
	c := Config{Doc: store}
	if err := c.SetResolverURL(ctx, " https://dns.example.org/q/ "); err != nil || view(c).ResolverURL != "https://dns.example.org/q" {
		t.Fatalf("url = %q, %v", view(c).ResolverURL, err)
	}
	refused := []string{"http://dns.example.org/q", "dns.example.org", "https://u:p@dns.example.org/q", "https://dns.example.org/q?x=1"}
	for _, bad := range refused {
		if err := c.SetResolverURL(ctx, bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if view(c).ResolverURL != "https://dns.example.org/q" {
		t.Errorf("a refused address must not replace the kept one: %q", view(c).ResolverURL)
	}
	if err := c.SetResolverURL(ctx, ""); err != nil || store.str("dns_check_resolver_url") != "" || view(c).ResolverURL != view(c).DefaultResolverURL {
		t.Errorf("empty goes back to the default: %q %v", store.str("dns_check_resolver_url"), err)
	}
	if err := c.SetResolverURL(ctx, view(c).DefaultResolverURL); err != nil || store.str("dns_check_resolver_url") != "" {
		t.Errorf("the default address is stored as nothing: %q %v", store.str("dns_check_resolver_url"), err)
	}
}

func TestConfig_TheRoutersDNSAddressIsCheckedAndEmptyMeansTheGateway(t *testing.T) {
	store := &memDoc{}
	c := Config{Doc: store}
	if view(c).Server != "" {
		t.Fatalf("by default the router is the default gateway: %q", view(c).Server)
	}
	if err := c.SetServer(ctx, " 192.168.1.53 "); err != nil || view(c).Server != "192.168.1.53" {
		t.Fatalf("server = %q, %v", view(c).Server, err)
	}
	for _, bad := range []string{"127.0.0.53", "8.8.8.8", "router.local", "192.168.1.53:53"} {
		if err := c.SetServer(ctx, bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if view(c).Server != "192.168.1.53" {
		t.Errorf("a refused address must not replace the kept one: %q", view(c).Server)
	}
	if err := c.SetServer(ctx, ""); err != nil || store.str("dns_check_server") != "" || view(c).Server != "" {
		t.Errorf("empty goes back to the gateway: %q %v", store.str("dns_check_server"), err)
	}
}

// Only this stack's five fields are named in a save, so the other settings in the document are never touched.
func TestConfig_ASaveNamesOnlyItsOwnFieldAndKeepsTheRest(t *testing.T) {
	store := &memDoc{fields: map[string]json.RawMessage{"presence_check": json.RawMessage("true"), "scan_ranges": json.RawMessage(`["192.168.1.0/24"]`)}}
	c := Config{Doc: store}
	if err := c.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if string(store.fields["presence_check"]) != "true" || string(store.fields["scan_ranges"]) != `["192.168.1.0/24"]` || string(store.fields["dns_check_enabled"]) != "true" {
		t.Errorf("document = %v", store.fields)
	}
}

// Settings that cannot be read count as off: nothing is sent.
func TestConfig_SettingsThatCannotBeReadAreOff(t *testing.T) {
	if s := (Config{Doc: brokenDoc{}}).Settings(ctx); s.Enabled || len(s.Names) != 0 {
		t.Errorf("settings = %+v", s)
	}
}

type brokenDoc struct{}

func (brokenDoc) Load(context.Context) (json.RawMessage, error) { return json.RawMessage("{"), nil }
func (brokenDoc) Save(context.Context, json.RawMessage) error   { return nil }
