package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestDeepChecks_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadDeepChecks("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no deep checks: %v, %v", got, err)
	}
	dc := models.DeepCheck{
		Device: "10.0.0.5", At: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC),
		Ports:    []models.DeepPortResult{{Port: 22, SSH: true}, {Port: 443}},
		Findings: []models.ScanFinding{{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: "ssh_password_login", Severity: "medium", Evidence: "x"}},
	}
	if err := mgr.WriteDeepChecks("ws", []models.DeepCheck{dc}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadDeepChecks("ws")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], dc) {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadDeepChecks("other-ws"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestDeepChecks_ACorruptFileIsKeptAndReported(t *testing.T) {
	mgr := newBaselineManager(t)
	path := mgr.deepChecksPath("ws")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReadDeepChecks("ws"); err == nil {
		t.Fatal("an undecodable file must be reported")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Errorf("the bad file is kept for the operator: %v", err)
	}
}
