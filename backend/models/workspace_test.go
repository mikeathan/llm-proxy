package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Automations live in the workspace's config.yaml. memory_mode and assistant_memory must survive a save
// and a reload, or an override would silently fall back to the global default the next time it is read.
func TestAutomation_MemoryModeSurvivesTheConfigYAML(t *testing.T) {
	cfg := WorkspaceConfig{AssistantMemory: MemoryModeOff, Automations: []*Automation{
		{Name: "recall", TaskFile: "recall.md", Trigger: TriggerConfig{Type: "manual"}, MemoryMode: MemoryModeOn},
		{Name: "plain", TaskFile: "plain.md", Trigger: TriggerConfig{Type: "manual"}},
	}}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`memory_mode: "on"`, `assistant_memory: "off"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing from the saved config:\n%s", want, data)
		}
	}
	if strings.Count(string(data), "memory_mode") != 1 {
		t.Errorf("an unset mode must be omitted, not written as an empty key:\n%s", data)
	}

	var back WorkspaceConfig
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.Automations[0].MemoryMode; got != MemoryModeOn {
		t.Errorf("reloaded memory_mode = %q, want on", got)
	}
	if back.AssistantMemory != MemoryModeOff {
		t.Errorf("reloaded assistant_memory = %q, want off", back.AssistantMemory)
	}
	if got := back.Automations[1].MemoryMode; got != MemoryModeInherit {
		t.Errorf("an automation without memory_mode must inherit, got %q", got)
	}
}

func TestAutomation_HandWrittenMemoryModeIsRead(t *testing.T) {
	var cfg WorkspaceConfig
	doc := "automations:\n  - name: recall\n    task_file: recall.md\n    memory_mode: on\n    trigger: {type: manual, value: \"\"}\n"
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Automations) != 1 || cfg.Automations[0].MemoryMode != MemoryModeOn {
		t.Errorf("hand-edited config.yaml not read: %+v", cfg.Automations)
	}
}

// A warning added after a run finished (a failed report delivery) must land on both copies of that run: the history
// entry and the automation's latest-run record, which AppendRun stores separately.
func TestAgentState_AddRunWarning(t *testing.T) {
	var s AgentState
	s.AppendRun(AutomationRun{ID: "run_1", AutomationName: "nightly"})
	s.AppendRun(AutomationRun{ID: "run_2", AutomationName: "nightly"})

	if !s.AddRunWarning("run_2", "report not delivered") {
		t.Fatal("AddRunWarning reported the run as missing")
	}
	if got := s.History[1].Warnings; len(got) != 1 || got[0] != "report not delivered" {
		t.Errorf("history entry warnings = %v", got)
	}
	if got := s.LastRuns["nightly"].Warnings; len(got) != 1 {
		t.Errorf("latest-run record warnings = %v", got)
	}
	if len(s.History[0].Warnings) != 0 {
		t.Error("another run was changed")
	}
	if s.AddRunWarning("run_404", "x") {
		t.Error("an unknown run id must report false")
	}
}
