package app

import (
	"os"
	"path/filepath"
	"testing"
)

// The marker is how a command-line tool can tell, from the data folder alone, that a live server reads that folder.
func TestServerMarker_WriteReadAliveRemove(t *testing.T) {
	root := t.TempDir()
	if _, ok := ReadServerMarker(root); ok {
		t.Fatal("no server has started here")
	}
	if err := WriteServerMarker(root, ":4001"); err != nil {
		t.Fatal(err)
	}
	m, ok := ReadServerMarker(root)
	if !ok || m.PID != os.Getpid() || m.Bind != ":4001" || m.StartedAt.IsZero() {
		t.Fatalf("marker = %+v %v", m, ok)
	}
	if !m.Alive() {
		t.Error("this process wrote it, so it is alive")
	}
	fi, _ := os.Stat(ServerMarkerPath(root))
	if fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("the marker is readable by others: %v", fi.Mode().Perm())
	}
	RemoveServerMarker(root)
	if _, ok := ReadServerMarker(root); ok {
		t.Error("the marker survived its removal")
	}
	RemoveServerMarker(root) // removing twice is fine
}

func TestServerMarker_AStaleOrDamagedMarkerIsNotAServer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(ServerMarkerPath(root), []byte(`{"pid": 2147483646, "bind": ":4001"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, ok := ReadServerMarker(root)
	if !ok || m.Alive() {
		t.Errorf("a marker whose process is gone must read as not alive: %+v %v", m, ok)
	}
	if err := os.WriteFile(ServerMarkerPath(root), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadServerMarker(root); ok {
		t.Error("a damaged marker must read as absent")
	}
	if filepath.Base(ServerMarkerPath(root)) != "server.json" {
		t.Errorf("name = %s", ServerMarkerPath(root))
	}
}
