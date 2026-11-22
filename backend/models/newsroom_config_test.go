package models

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// Each module's switch is its own "off" flag in settings.yml, so a fresh install has both on and switching one never moves the other.
func TestNewsroomConfig_EachSwitchSurvivesTheSettingsFile(t *testing.T) {
	cfg := AppConfig{Newsroom: &NewsroomConfig{Timezone: "Europe/London", BriefsDisabled: true, ModelCallsPerDay: 400}}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back AppConfig
	if err := yaml.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Newsroom == nil || !back.Newsroom.BriefsDisabled || back.Newsroom.Disabled || back.Newsroom.ModelCallsPerDay != 400 || back.Newsroom.Timezone != "Europe/London" {
		t.Errorf("yaml round trip: %+v from %s", back.Newsroom, raw)
	}
	var fromJSON NewsroomConfig
	if err := json.Unmarshal([]byte(`{"disabled":true,"briefs_disabled":false}`), &fromJSON); err != nil || !fromJSON.Disabled || fromJSON.BriefsDisabled {
		t.Errorf("json: %+v, %v", fromJSON, err)
	}
	if raw, _ := yaml.Marshal(NewsroomConfig{}); string(raw) != "{}\n" {
		t.Errorf("a fresh install writes no flags, so both modules are on: %q", raw)
	}
}
