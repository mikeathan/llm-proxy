package storage

import (
	"reflect"
	"testing"

	"llm-proxy/models"
)

func TestAgentsView_EveryPartIsOnWhenNothingIsStored(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if got := mgr.Agents().Get(); got.ActivityOff || got.SecretsPassOff {
		t.Errorf("an install with nothing stored must keep both parts on, got %+v", got)
	}
}

func TestAgentsView_UpdateSurvivesAnOrdinarySettingsSaveAndAReload(t *testing.T) {
	mgr, _ := resetTestManager(t)
	want := models.AgentsConfig{ActivityOff: true}
	if err := mgr.Agents().Update(func(c *models.AgentsConfig) error { *c = want; return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mgr.Settings().Update(func(s *models.UserSettings) error { s.Local.ModelDir = "/models"; return nil }); err != nil {
		t.Fatalf("Settings().Update: %v", err)
	}
	if got := mgr.Agents().Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("after a settings save = %+v, want %+v", got, want)
	}
}

func TestAgentsView_ARefusedUpdateChangesNothing(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if err := mgr.Agents().Update(func(c *models.AgentsConfig) error { c.ActivityOff = true; return errTestUpdate }); err == nil {
		t.Fatal("Update must return the refusal")
	}
	if got := mgr.Agents().Get(); got.ActivityOff {
		t.Errorf("a refused update was applied: %+v", got)
	}
}
