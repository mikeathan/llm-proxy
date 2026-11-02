package hostwatch

import (
	"os"
	"path/filepath"
	"testing"
)

func keyFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	write(t, path, "not a real key\n")
	if err := os.Chmod(path, mode); err != nil { // WriteFile's mode is cut by the umask
		t.Fatal(err)
	}
}

func openKeys(t *testing.T, globs ...string) Hardening {
	t.Helper()
	return Hardener{KeyGlobs: globs}.Check()
}

func TestKeyFiles_OnlyOwnerAccessIsFine(t *testing.T) {
	dir := t.TempDir()
	keyFile(t, filepath.Join(dir, "id_ed25519"), 0o600)
	keyFile(t, filepath.Join(dir, "id_rsa"), 0o400)
	if got := openKeys(t, filepath.Join(dir, "id_*")); got.OpenKeyFiles != 0 {
		t.Errorf("owner-only keys are not open: %+v", got)
	}
}

func TestKeyFiles_GroupOrWorldAccessIsCounted(t *testing.T) {
	dir := t.TempDir()
	keyFile(t, filepath.Join(dir, "id_ed25519"), 0o644)
	keyFile(t, filepath.Join(dir, "id_rsa"), 0o640)
	keyFile(t, filepath.Join(dir, "id_ecdsa"), 0o600)
	if got := openKeys(t, filepath.Join(dir, "id_*")); got.OpenKeyFiles != 2 {
		t.Errorf("want 2 open keys, got %+v", got)
	}
}

func TestKeyFiles_PublicKeysAndMissingFoldersAreIgnored(t *testing.T) {
	dir := t.TempDir()
	keyFile(t, filepath.Join(dir, "id_ed25519.pub"), 0o644) // a public key is meant to be shared
	got := openKeys(t, filepath.Join(dir, "id_*"), filepath.Join(dir, "nowhere", "id_*"))
	if got.OpenKeyFiles != 0 || got.Unreadable != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestKeyFiles_ALinkIsJudgedByItsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real_key")
	keyFile(t, target, 0o600)
	if err := os.Symlink(target, filepath.Join(dir, "id_link")); err != nil { // a link's own mode is always 0777
		t.Fatal(err)
	}
	if got := openKeys(t, filepath.Join(dir, "id_*")); got.OpenKeyFiles != 0 {
		t.Errorf("a link to a private file is not open: %+v", got)
	}
}
