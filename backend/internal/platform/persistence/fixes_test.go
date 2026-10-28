package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestFixes_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadFixes("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no records: %v, %v", got, err)
	}
	rec := models.FixRecord{Key: "10.0.0.1|tcp|445|smb_v1", AppliedAt: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC), Note: "turned it off"}
	if err := mgr.WriteFixes("ws", []models.FixRecord{rec}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadFixes("ws")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], rec) {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadFixes("other-ws"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestFixes_ACorruptFileIsKeptAndReported(t *testing.T) {
	mgr := newBaselineManager(t)
	path := mgr.fixesPath("ws")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReadFixes("ws"); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the bad file must be kept: %v", err)
	}
}

func TestFixes_DeletingTheWorkspaceRemovesThem(t *testing.T) {
	mgr := newBaselineManager(t)
	if err := mgr.WriteFixes("ws", []models.FixRecord{{Key: "k"}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace("ws"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadFixes("ws"); len(got) != 0 {
		t.Errorf("fix records outlived the workspace: %+v", got)
	}
}
