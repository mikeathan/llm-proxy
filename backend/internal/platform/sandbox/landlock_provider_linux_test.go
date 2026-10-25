//go:build linux

package sandbox

import (
	"os/exec"
	"strings"
	"testing"
)

// filesystemMechanismAvailable reports whether this host selects a real
// filesystem mechanism (test-only seam, hence a _test.go file).
func filesystemMechanismAvailable() bool {
	_, err := landlockABI()
	return err == nil
}

// Child-side parse cost of the serialized profile (Linux runner path).
func BenchmarkDecodeProfile_linux(b *testing.B) {
	p := Profile{Rules: make([]Rule, 0, 64)}
	for i := 0; i < 64; i++ {
		p.Rules = append(p.Rules, Rule{Path: "/workspace/dir" + string(rune('a'+i%26)) + "/sub/deeper", Perm: PermRead | PermWrite})
	}
	enc := EncodeProfile(p)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeProfile(enc); err != nil {
			b.Fatal(err)
		}
	}
}

// A jailed child may write /dev/null: nmap refuses to start when it cannot, and a shell redirect to it must work. Every
// other /dev node it is given stays read-only. The grants are read back from what Wrap hands the runner.
func TestLandlockWrapGrantsDevNullReadWrite(t *testing.T) {
	if !filesystemMechanismAvailable() {
		t.Skip("no Landlock on this host")
	}
	cmd := exec.Command("true")
	if err := (landlockProvider{}).Wrap(cmd, WorkspaceView{RootPath: t.TempDir(), NetworkOn: true}); err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	var encoded string
	for _, kv := range cmd.Env {
		if v, ok := strings.CutPrefix(kv, rulesEnvVar+"="); ok {
			encoded = v
		}
	}
	profile, err := DecodeProfile(encoded)
	if err != nil {
		t.Fatalf("DecodeProfile: %v", err)
	}
	var devNull, devZero FSPermission
	for _, r := range profile.Rules {
		switch r.Path {
		case "/dev/null":
			devNull |= r.Perm
		case "/dev/zero":
			devZero |= r.Perm
		}
	}
	if devNull&PermWrite == 0 || devNull&PermRead == 0 {
		t.Errorf("/dev/null must be readable and writable, got perm %b", devNull)
	}
	if devZero&PermWrite != 0 {
		t.Errorf("/dev/zero must stay read-only, got perm %b", devZero)
	}
}
