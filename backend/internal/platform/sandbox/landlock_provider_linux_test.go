//go:build linux

package sandbox

import "testing"

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
