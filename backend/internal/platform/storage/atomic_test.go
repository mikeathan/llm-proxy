package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic_success(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	if err := WriteAtomic(dest, "test-*.tmp", []byte(`{"ok":true}`), ClassData); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest file not created: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("content = %q, want %q", data, `{"ok":true}`)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file in dir, got %d: %v", len(entries), entries)
	}
}

func TestWriteAtomic_createsDir(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "deep", "test.json")

	if err := WriteAtomic(dest, "test-*.tmp", []byte("hello"), ClassData); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest file not created: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}
}

func TestWriteAtomic_noTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")

	if err := WriteAtomic(dest, "out-*.tmp", []byte("data"), ClassData); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestWriteAtomic_failureLeavesNoDest(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "ro")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Make the target directory read-only so CreateTemp fails, exercising the
	// early error path. Restore permissions so t.TempDir cleanup succeeds.
	t.Cleanup(func() { os.Chmod(sub, 0o755) })
	if err := os.Chmod(sub, 0o500); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}

	dest := filepath.Join(sub, "out.txt")
	err := WriteAtomic(dest, "out-*.tmp", []byte("data"), ClassData)
	if err == nil {
		t.Fatal("expected error on read-only dir, got nil")
	}

	// The destination must not be created on failure.
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Errorf("dest file unexpectedly created on failure: %s", dest)
	}
}

// TestWriteAtomic_dirAndFileModes asserts S1: dirs are created at dirPerm and
// the resulting file is 0600 (CreateTemp), never world-readable.
func TestWriteAtomic_dirAndFileModes(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "secret.json")
	if err := WriteAtomic(dest, "s-*.tmp", []byte("x"), ClassSecret); err != nil {
		t.Fatal(err)
	}

	dfi, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if dfi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 0700", dfi.Mode().Perm())
	}
	ffi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if ffi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 0600", ffi.Mode().Perm())
	}
}

// TestWriteAtomic_tightensExistingDir asserts S1: MkdirAll does not tighten an
// existing open directory; WriteAtomic must.
func TestWriteAtomic_tightensExistingDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(sub, "out.json")
	if err := WriteAtomic(dest, "t-*.tmp", []byte("x"), ClassData); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(sub)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("existing dir mode = %o, want tightened to 0700", fi.Mode().Perm())
	}
}

// TestFileClassPolicy asserts the canonical permission table: managed classes are
// 0700/0600, user content is relaxed to 0644. This locks the single source of
// truth so a future edit cannot silently weaken a secret tier.
func TestFileClassPolicy(t *testing.T) {
	cases := []struct {
		class        FileClass
		wantDirPerm  os.FileMode
		wantFilePerm os.FileMode
	}{
		{ClassSecret, 0o700, 0o600},
		{ClassConfig, 0o700, 0o600},
		{ClassData, 0o700, 0o600},
		{ClassUserContent, 0o700, 0o644},
	}
	for _, c := range cases {
		if got := c.class.dirMode(); got != c.wantDirPerm {
			t.Errorf("%d dir mode = %o, want %o", c.class, got, c.wantDirPerm)
		}
		if got := c.class.fileMode(); got != c.wantFilePerm {
			t.Errorf("%d file mode = %o, want %o", c.class, got, c.wantFilePerm)
		}
	}
}

// TestWriteAtomic_userContentFileMode asserts ClassUserContent yields 0644 on the
// final file, exercising the post-rename chmod that CreateTemp's 0600 would
// otherwise leave in place.
func TestWriteAtomic_userContentFileMode(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "state.json")
	if err := WriteAtomic(dest, "s-*.tmp", []byte("x"), ClassUserContent); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("user content file mode = %o, want 0644", fi.Mode().Perm())
	}
}
