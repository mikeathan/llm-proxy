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

func TestRadioView_TheScanPlanSurvivesAReloadAndAnOrdinarySettingsSave(t *testing.T) {
	mgr, _ := resetTestManager(t)
	err := mgr.Radio().Update(func(c *models.RadioConfig) error {
		c.Enabled, c.Channels, c.WindowSeconds, c.CycleSeconds = true, []int{1, 6, 11}, 10, 120
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mgr.Settings().Update(func(s *models.UserSettings) error { s.Local.ModelDir = "/models"; return nil }); err != nil {
		t.Fatalf("Settings().Update: %v", err)
	}
	got := mgr.Radio().Get()
	if len(got.Channels) != 3 || got.Channels[2] != 11 || got.WindowSeconds != 10 || got.CycleSeconds != 120 || !got.Enabled {
		t.Errorf("the scan plan was lost: %+v", got)
	}
}
