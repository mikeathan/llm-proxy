package models

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// server.auth in settings.yml: lifetimes in whole hours and days, zero meaning the default, so a settings file written
// before the keys existed loads as it did.
func TestAuthSettings_YAML(t *testing.T) {
	var cfg AppServerConfig
	if err := yaml.Unmarshal([]byte("bind: ':4001'\nauth:\n  idle_hours: 12\n  absolute_days: 30\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.IdleHours != 12 || cfg.Auth.AbsoluteDays != 30 {
		t.Errorf("auth = %+v", cfg.Auth)
	}
	var old AppServerConfig
	if err := yaml.Unmarshal([]byte("bind: ':4001'\n"), &old); err != nil {
		t.Fatal(err)
	}
	if old.Auth != (AuthSettings{}) {
		t.Errorf("a file with no auth keys must read as all defaults: %+v", old.Auth)
	}
	out, err := yaml.Marshal(AppServerConfig{Bind: ":4001"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) == "" || containsKey(string(out), "auth") {
		t.Errorf("unset lifetimes must not be written out: %s", out)
	}
}

func containsKey(doc, key string) bool {
	var m map[string]any
	_ = yaml.Unmarshal([]byte(doc), &m)
	_, ok := m[key]
	return ok
}
