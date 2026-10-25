package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestAcks_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadAcks("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no acknowledgements: %v, %v", got, err)
	}
	ack := models.ScanAck{Key: "10.0.0.1|tcp|22|open_port", AcceptedVersion: "OpenSSH 9", Note: "jump host", AcceptedAt: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2027, 1, 25, 0, 0, 0, 0, time.UTC)}
	if err := mgr.WriteAcks("ws", []models.ScanAck{ack}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadAcks("ws")
	if err != nil || len(got) != 1 || got[0] != ack {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadAcks("other-ws"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestAcks_ACorruptFileIsKeptAndReported(t *testing.T) {
	mgr := newBaselineManager(t)
	path := mgr.acksPath("ws")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReadAcks("ws"); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the bad file must be kept: %v", err)
	}
}

func TestAcks_DeletingTheWorkspaceRemovesThem(t *testing.T) {
	mgr := newBaselineManager(t)
	if err := mgr.WriteAcks("ws", []models.ScanAck{{Key: "k"}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace("ws"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadAcks("ws"); len(got) != 0 {
		t.Errorf("acknowledgements outlived the workspace: %+v", got)
	}
}
