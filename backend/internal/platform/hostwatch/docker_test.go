package hostwatch

import (
	"os"
	"path/filepath"
	"testing"
)

func netDirWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDockerBridge_IsSeenByTheDefaultBridgeOrAUserBridge(t *testing.T) {
	cases := map[string][]string{
		"default bridge": {"lo", "wlp2s0", "docker0"},
		"user bridge":    {"lo", "br-3f9a1c2d4e5b"},
	}
	for name, ifaces := range cases {
		if !(Docker{NetDir: netDirWith(t, ifaces...)}).Bridge() {
			t.Errorf("%s: want a Docker bridge", name)
		}
	}
}

func TestDockerBridge_IsNotGuessedFromOtherInterfacesOrAMissingDirectory(t *testing.T) {
	if (Docker{NetDir: netDirWith(t, "lo", "wlp2s0", "bridge0", "brx")}).Bridge() {
		t.Error("an interface that only resembles a bridge name is not Docker's")
	}
	if (Docker{NetDir: filepath.Join(t.TempDir(), "missing")}).Bridge() {
		t.Error("a directory that cannot be read says nothing about Docker, so it reports no bridge")
	}
	if (Docker{}).Bridge() {
		t.Error("no directory configured reports no bridge")
	}
}
