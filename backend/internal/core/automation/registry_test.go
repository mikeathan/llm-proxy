package automation

import (
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
