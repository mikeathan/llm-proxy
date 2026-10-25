package storage

import (
	"errors"
	"reflect"
	"testing"

	"llm-proxy/models"
)

var errTestUpdate = errors.New("refused")

func TestNetauditView_DefaultsToScanningOff(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if got := mgr.Netaudit().Get(); len(got.ScanRanges) != 0 || got.AllowUnconfined {
		t.Errorf("a fresh install must have scanning off, got %+v", got)
	}
}

func TestNetauditView_UpdateSurvivesReloadAndOrdinarySettingsSaves(t *testing.T) {
	mgr, _ := resetTestManager(t)
	want := models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}, AllowUnconfined: true}

	if err := mgr.Netaudit().Update(func(c *models.NetauditConfig) error { *c = want; return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Saving the ordinary settings must not touch the scan policy.
	if err := mgr.Settings().Update(func(s *models.UserSettings) error { s.Local.ModelDir = "/models"; return nil }); err != nil {
		t.Fatalf("Settings().Update: %v", err)
	}
	if got := mgr.Netaudit().Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("after a settings save = %+v, want %+v", got, want)
	}

	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if got := mgr.Netaudit().Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("after reload = %+v, want %+v", got, want)
	}
}

func TestNetauditView_FailedUpdateChangesNothing(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if err := mgr.Netaudit().Update(func(c *models.NetauditConfig) error {
		c.ScanRanges = []string{"10.0.0.0/24"}
		return errTestUpdate
	}); err == nil {
		t.Fatal("the update's error must be returned")
	}
	if got := mgr.Netaudit().Get(); len(got.ScanRanges) != 0 {
		t.Errorf("a refused update must leave the policy unchanged, got %+v", got)
	}
}
