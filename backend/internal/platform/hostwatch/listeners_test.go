package hostwatch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// procRoot builds a /proc look-alike from the captured tables. The service's own sockets (uid 995 in the capture) are linked
// from self/fd the way the kernel does it.
func procRoot(t *testing.T, ephemeralLow string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tcp", "tcp6", "udp", "udp6"} {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "net", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fd := filepath.Join(root, "self", "fd")
	if err := os.MkdirAll(fd, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, inode := range []string{"2127412", "2127416", "2127414", "2129635"} {
		if err := os.Symlink("socket:["+inode+"]", filepath.Join(fd, string(rune('3'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/dev/null", filepath.Join(fd, "0")); err != nil {
		t.Fatal(err)
	}
	if ephemeralLow != "" {
		dir := filepath.Join(root, "sys", "net", "ipv4")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ip_local_port_range"), []byte(ephemeralLow+"\t60999\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestListeners_RealCaptureKeepsOnlyWhatOthersCanReach(t *testing.T) {
	got, err := Proc{Root: procRoot(t, "32768")}.Listeners()
	if err != nil {
		t.Fatal(err)
	}
	// Loopback binds, the service's own sockets (inodes under self/fd), UDP ports in the ephemeral range and the duplicate
	// IPv6 rows are all left out; what remains is ssh, the root-owned 3552 and mDNS.
	want := []Listener{{"tcp", 22}, {"tcp", 3552}, {"udp", 5353}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listener %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListeners_UsesTheHostsEphemeralRange(t *testing.T) {
	// With the range starting above 52039 the avahi ports are ordinary services, so they are reported.
	got, err := Proc{Root: procRoot(t, "60000")}.Listeners()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("got %+v, want ssh, 3552, and udp 5353, 41849, 52039", got)
	}
}

func TestListeners_NoProcIsUnavailableNotEmpty(t *testing.T) {
	_, err := Proc{Root: t.TempDir()}.Listeners()
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable: a computer that cannot be read must not look like one with nothing listening", err)
	}
}
