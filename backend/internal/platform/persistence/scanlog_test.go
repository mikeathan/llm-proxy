package persistence

import (
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestScanLog_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadScanLog("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no rows: %v, %v", got, err)
	}
	row := models.ScanEntry{Automation: "a", At: time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC), DevicesUp: 15, OpenPorts: 34, NewKeys: []string{"k"}}
	if err := mgr.UpdateScanLog("ws", func(rows []models.ScanEntry) []models.ScanEntry { return append(rows, row) }); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadScanLog("ws")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], row) {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadScanLog("other"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestScanLog_DeletingTheWorkspaceRemovesIt(t *testing.T) {
	mgr := newBaselineManager(t)
	if err := mgr.WriteScanLog("ws", []models.ScanEntry{{Automation: "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace("ws"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadScanLog("ws"); len(got) != 0 {
		t.Errorf("the scan log must go with the workspace: %+v", got)
	}
}
