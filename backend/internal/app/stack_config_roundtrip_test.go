package app

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// fillNonZero gives every field a value that is not the zero value, so a field the adapter loses or renames shows up as a difference.
func fillNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(15)
	case reflect.String:
		v.SetString("x-" + strings.ToLower(v.Type().Name()))
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillNonZero(t, s.Index(0))
		v.Set(s)
	case reflect.Struct:
		for i := range v.NumField() {
			fillNonZero(t, v.Field(i))
		}
	default:
		t.Fatalf("fillNonZero: no rule for %s: add one so the round-trip test keeps covering every field", v.Type())
	}
}

// The Config adapter is the one place a user's Network settings could be lost (plan P3b section 4c, finding 2). An existing settings.yml
// must load and save through it byte for byte, for every field the settings hold, and a partial document must leave the rest alone.
func TestStackConfig_ASettingsFileRoundTripsByteForByteForEveryField(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatal(err)
	}
	mgr, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	var full models.NetauditConfig
	fillNonZero(t, reflect.ValueOf(&full).Elem())
	if err := mgr.Netaudit().Update(func(c *models.NetauditConfig) error { *c = full; return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}

	cfg := stackport.Config[models.NetauditConfig]{Store: mgr.Netaudit()}
	doc, err := cfg.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("settings.yml changed through the adapter:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if got := mgr.Netaudit().Get(); !reflect.DeepEqual(got, full) {
		t.Errorf("a field was lost or changed:\n got %+v\nwant %+v", got, full)
	}

	if err := cfg.Save(context.Background(), []byte(`{"dns_check_enabled":false}`)); err != nil {
		t.Fatal(err)
	}
	want := full
	want.DNSCheckEnabled = false
	if got := mgr.Netaudit().Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("a partial document changed more than it named:\n got %+v\nwant %+v", got, want)
	}
}
