package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Automations live in the workspace's config.yaml. memory_mode must survive a save
// and a reload, or an automation opted into hot memory would silently lose it the
// next time the config is read.
func TestAutomation_MemoryModeSurvivesTheConfigYAML(t *testing.T) {
	cfg := WorkspaceConfig{Automations: []*Automation{
		{Name: "recall", TaskFile: "recall.md", Trigger: TriggerConfig{Type: "manual"}, MemoryMode: MemoryModeHot},
		{Name: "plain", TaskFile: "plain.md", Trigger: TriggerConfig{Type: "manual"}},
	}}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "memory_mode: hot") {
		t.Errorf("hot mode missing from the saved config:\n%s", data)
	}
	if strings.Count(string(data), "memory_mode") != 1 {
		t.Errorf("an unset mode must be omitted, not written as an empty key:\n%s", data)
	}

	var back WorkspaceConfig
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.Automations[0].MemoryMode; got != MemoryModeHot {
		t.Errorf("reloaded memory_mode = %q, want hot", got)
	}
	if back.Automations[1].MemoryMode.HotEnabled() {
		t.Error("an automation without memory_mode must stay off")
	}
}

func TestAutomation_HandWrittenMemoryModeIsRead(t *testing.T) {
	var cfg WorkspaceConfig
	doc := "automations:\n  - name: recall\n    task_file: recall.md\n    memory_mode: hot\n    trigger: {type: manual, value: \"\"}\n"
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Automations) != 1 || !cfg.Automations[0].MemoryMode.HotEnabled() {
		t.Errorf("hand-edited config.yaml not read: %+v", cfg.Automations)
	}
}
