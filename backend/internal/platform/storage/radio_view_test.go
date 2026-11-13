package storage

import (
	"testing"

	"llm-proxy/models"
)

func TestRadioView_IsOffWhenNothingIsStored(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if got := mgr.Radio().Get(); got.Enabled {
		t.Errorf("an install that never opted in must have the Radio stack off, got %+v", got)
	}
}

func TestRadioView_UpdateSurvivesAnOrdinarySettingsSave(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if err := mgr.Radio().Update(func(c *models.RadioConfig) error { c.Enabled = true; return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mgr.Settings().Update(func(s *models.UserSettings) error { s.Local.ModelDir = "/models"; return nil }); err != nil {
		t.Fatalf("Settings().Update: %v", err)
	}
	if got := mgr.Radio().Get(); !got.Enabled {
		t.Errorf("after a settings save = %+v, want enabled", got)
	}
}

func TestRadioView_ARefusedUpdateChangesNothing(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if err := mgr.Radio().Update(func(c *models.RadioConfig) error { c.Enabled = true; return errTestUpdate }); err == nil {
		t.Fatal("Update must return the refusal")
	}
	if got := mgr.Radio().Get(); got.Enabled {
		t.Errorf("a refused update was applied: %+v", got)
	}
}
