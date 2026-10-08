package models

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestHeartbeatBody(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n\t\n", ""},
		{"comment only", "<!-- what to watch -->", ""},
		{"multi-line comments and blank lines", "<!--\n# Heartbeat\nAdd checks here\n-->\n\n<!-- importance bar -->\n", ""},
		{"a check after the comments", "<!-- note -->\nWatch: new Claude releases\n", "Watch: new Claude releases"},
		{"comment between checks", "One\n<!-- hidden -->\nTwo", "One\n\nTwo"},
		{"unterminated comment hides the rest", "Keep\n<!-- never closed\nDrop", "Keep"},
		{"a comment marker inside a code fence is content", "```\n<!-- literal -->\n```", "```\n<!-- literal -->\n```"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HeartbeatBody(c.in); got != c.want {
				t.Errorf("HeartbeatBody(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestHeartbeatConfig_Validate(t *testing.T) {
	cases := []struct {
		every   string
		wantErr bool
	}{
		{"", false}, // unset takes the default
		{"5m", false},
		{"30m", false},
		{"6h", false},
		{"30s", true}, // faster than the minimum
		{"25h", true}, // slower than a day
		{"soon", true},
		{"-5m", true},
	}
	for _, c := range cases {
		err := HeartbeatConfig{Every: c.every}.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("Validate(every=%q) error = %v, wantErr %v", c.every, err, c.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "heartbeat.every") {
			t.Errorf("error should name the field: %v", err)
		}
	}
}

func TestHeartbeatConfig_Automation(t *testing.T) {
	notify := &NotifyConfig{Connector: "my-telegram"}
	auto := HeartbeatConfig{Enabled: true, Every: "15m", Model: "gpt-5", Notify: notify}.Automation()
	if auto.Name != HeartbeatAutomationName || auto.TaskFile != HeartbeatFilename {
		t.Errorf("name/task = %q/%q, want %q/%q", auto.Name, auto.TaskFile, HeartbeatAutomationName, HeartbeatFilename)
	}
	if auto.Trigger != (TriggerConfig{Type: TriggerInterval, Value: "15m"}) {
		t.Errorf("trigger = %+v", auto.Trigger)
	}
	if auto.Model != "gpt-5" || auto.Notify != notify || auto.Strategy != "isolated" {
		t.Errorf("model/notify/strategy = %q/%v/%q", auto.Model, auto.Notify, auto.Strategy)
	}
	if auto.MemoryMode != MemoryModeOff {
		t.Errorf("memory = %q: a check that runs all day must never pay for hot memory", auto.MemoryMode)
	}
	if got := (HeartbeatConfig{}).Automation().Trigger.Value; got != "30m" {
		t.Errorf("unset interval = %q, want the 30m default", got)
	}
}

// A workspace file written before the heartbeat section existed (with the old cron field) still loads.
func TestHeartbeatConfig_ValidateActiveHours(t *testing.T) {
	for _, c := range []struct {
		hours   string
		wantErr bool
	}{
		{"", false}, // unset means always active
		{"08:00-22:00", false},
		{"22:00-06:00", false},
		{"08:00-08:00", true},
		{"late", true},
	} {
		err := HeartbeatConfig{ActiveHours: c.hours}.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("Validate(active_hours=%q) error = %v, wantErr %v", c.hours, err, c.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "heartbeat.active_hours") {
			t.Errorf("error should name the field: %v", err)
		}
	}
}

func TestHeartbeatConfig_ActiveAt(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 8, h, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		name  string
		hours string
		hour  int
		want  bool
	}{
		{"no window is always active", "", 3, true},
		{"inside the window", "08:00-22:00", 12, true},
		{"outside the window", "08:00-22:00", 3, false},
		{"an unparsable window never silences the heartbeat", "garbage", 3, true},
	} {
		if got := (HeartbeatConfig{ActiveHours: c.hours}).ActiveAt(at(c.hour)); got != c.want {
			t.Errorf("%s: ActiveAt = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWorkspaceConfig_HeartbeatYAML(t *testing.T) {
	cfg := WorkspaceConfig{Heartbeat: &HeartbeatConfig{Enabled: true, Every: "1h", Model: "gpt-5", ActiveHours: "08:00-22:00"}}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back WorkspaceConfig
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Heartbeat == nil || *back.Heartbeat != *cfg.Heartbeat {
		t.Errorf("heartbeat did not survive the config YAML: %+v", back.Heartbeat)
	}

	var legacy WorkspaceConfig
	if err := yaml.Unmarshal([]byte("cron_schedule: \"0 * * * *\"\nmodel: qwen\n"), &legacy); err != nil {
		t.Fatalf("a config with the removed cron_schedule key must still load: %v", err)
	}
	if legacy.Model != "qwen" || legacy.Heartbeat != nil {
		t.Errorf("legacy config read as %+v", legacy)
	}
}
