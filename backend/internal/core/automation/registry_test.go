package automation

import (
	"context"
	"testing"

	"llm-proxy/models"
)

func TestRegistry_RegisterCarriesMemoryMode(t *testing.T) {
	reg := NewAutomationRegistry()
	auto := &models.Automation{
		Name:       "nightly",
		TaskFile:   "task.md",
		Trigger:    models.TriggerConfig{Type: "manual"},
		MemoryMode: models.MemoryModeOn,
	}
	if err := reg.Register("ws", auto); err != nil {
		t.Fatalf("Register: %v", err)
	}
	entry, ok := reg.Get("ws", "nightly")
	if !ok {
		t.Fatal("automation not registered")
	}
	if entry.MemoryMode != models.MemoryModeOn {
		t.Errorf("entry.MemoryMode = %q, want hot", entry.MemoryMode)
	}
}

func TestRegistry_RegisterCarriesTheEffectiveScanSettings(t *testing.T) {
	reg := NewAutomationRegistry()
	register := func(name, profile string, scan *models.ScanSettings) *AutomationEntry {
		t.Helper()
		if err := reg.Register("ws", &models.Automation{Name: name, TaskFile: "t.md", Trigger: models.TriggerConfig{Type: "manual"}, Profile: profile, Scan: scan}); err != nil {
			t.Fatal(err)
		}
		entry, _ := reg.Get("ws", name)
		return entry
	}
	if e := register("own", models.AuditorProfileID, &models.ScanSettings{Depth: models.ScanDepthQuick, Ports: "22"}); e.Scan == nil || e.Scan.Depth != models.ScanDepthQuick || e.Scan.Ports != "22" {
		t.Errorf("an audit's own settings reach its entry: %+v", e.Scan)
	}
	if e := register("old", models.AuditorProfileID, nil); e.Scan == nil || e.Scan.Depth != models.ScanDepthThorough || !e.Scan.ChecksOn() {
		t.Errorf("an audit saved before it had settings still gets the defaults, so the model never decides: %+v", e.Scan)
	}
	if e := register("plain", "research", nil); e.Scan != nil {
		t.Errorf("another agent's automation has no settings: %+v", e.Scan)
	}
}

func TestDispatcher_TheRunRequestAndContextCarryTheScanSettings(t *testing.T) {
	exec := &scriptedExecutor{report: "ok"}
	d, entry := newNotifyDispatcher(t, exec, &fakeNotifier{}, nil)
	entry.Scan = &models.ScanSettings{Depth: models.ScanDepthQuick}
	if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
		t.Fatal(err)
	}
	if got := exec.reqs[0].Scan; got == nil || got.Depth != models.ScanDepthQuick {
		t.Errorf("ExecuteRequest.Scan must carry the entry's settings, got %+v", got)
	}
}
