package hostwatch

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashOf(t *testing.T, spec FileSpec) FileState {
	t.Helper()
	got := Files{Specs: []FileSpec{spec}}.Observe()
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	return got[0]
}

func TestObserve_SameContentSameHashChangedContentNewHash(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "sshd_config"), "PermitRootLogin no\n")
	spec := FileSpec{ID: "ssh_config", Paths: []string{filepath.Join(dir, "sshd_config")}}
	first, again := hashOf(t, spec), hashOf(t, spec)
	if first.Hash == "" || first.Hash != again.Hash {
		t.Fatalf("an unchanged file must hash the same: %q vs %q", first.Hash, again.Hash)
	}
	write(t, filepath.Join(dir, "sshd_config"), "PermitRootLogin yes\n")
	if hashOf(t, spec).Hash == first.Hash {
		t.Error("a changed file must hash differently")
	}
}

func TestObserve_MissingIsAStateOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	spec := FileSpec{ID: "sudoers", Paths: []string{filepath.Join(dir, "sudoers")}}
	absent := hashOf(t, spec)
	write(t, filepath.Join(dir, "sudoers"), "x ALL=(ALL) ALL\n")
	if hashOf(t, spec).Hash == absent.Hash {
		t.Error("a file that appears must change the hash")
	}
	if absent.Unreadable != 0 {
		t.Errorf("a missing file is not an unreadable one: %+v", absent)
	}
}

func TestObserve_UnreadableIsCountedAndSteady(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sudoers")
	write(t, path, "secret\n")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	spec := FileSpec{ID: "sudoers", Paths: []string{path}}
	a := hashOf(t, spec)
	if a.Unreadable != 1 {
		t.Errorf("Unreadable = %d, want 1", a.Unreadable)
	}
	if b := hashOf(t, spec); b.Hash != a.Hash {
		t.Error("a file nobody here can read must not look changed from one look to the next")
	}
}

func TestObserve_DirectoryNoticesAddedFilesAndNewSymlinksInSubfolders(t *testing.T) {
	dir := t.TempDir()
	units := filepath.Join(dir, "system")
	write(t, filepath.Join(units, "app.service"), "[Service]\n")
	spec := FileSpec{ID: "systemd_units", Paths: []string{units}}
	before := hashOf(t, spec).Hash
	write(t, filepath.Join(units, "evil.service"), "[Service]\n")
	added := hashOf(t, spec).Hash
	if added == before {
		t.Error("a new unit file must change the hash")
	}
	// systemctl enable adds a symlink in a *.wants folder: one level down.
	wants := filepath.Join(units, "multi-user.target.wants")
	if err := os.MkdirAll(wants, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/systemd/system/app.service", filepath.Join(wants, "app.service")); err != nil {
		t.Fatal(err)
	}
	if hashOf(t, spec).Hash == added {
		t.Error("an enabled unit (a symlink one folder down) must change the hash")
	}
}

func TestObserve_ASymlinkIsHashedByItsTargetNotFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	write(t, target, "one")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	spec := FileSpec{ID: "x", Paths: []string{link}}
	before := hashOf(t, spec).Hash
	write(t, target, "two")
	if hashOf(t, spec).Hash != before {
		t.Error("the content behind a symlink is not followed")
	}
}

func TestObserve_GlobFindsAccountsAndProgramCarriesItsTag(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "alice", ".ssh", "authorized_keys"), "key-a")
	spec := FileSpec{ID: "authorized_keys", Globs: []string{filepath.Join(home, "*", ".ssh", "authorized_keys")}}
	before := hashOf(t, spec).Hash
	write(t, filepath.Join(home, "bob", ".ssh", "authorized_keys"), "key-b")
	if hashOf(t, spec).Hash == before {
		t.Error("a key file for another account must change the hash")
	}
	bin := filepath.Join(t.TempDir(), "llm-proxy")
	write(t, bin, "elf")
	got := Files{Specs: []FileSpec{{ID: "program", Paths: []string{bin}, Program: true}}, ProgramTag: "0.32.0"}.Observe()
	if got[0].Tag != "0.32.0" {
		t.Errorf("Tag = %q: the program's hash is compared only within one version", got[0].Tag)
	}
}

func TestObserve_ReadsOnlyUpToTheCap(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big")
	body := make([]byte, 3<<20)
	write(t, big, string(body))
	spec := FileSpec{ID: "x", Paths: []string{big}, MaxBytes: 1 << 20}
	before := hashOf(t, spec).Hash

	body[2<<20] = 1 // past the cap, same length: deliberately not seen
	write(t, big, string(body))
	if hashOf(t, spec).Hash != before {
		t.Error("bytes past the cap must not be read")
	}
	body[10] = 1 // inside the cap
	write(t, big, string(body))
	if hashOf(t, spec).Hash == before {
		t.Error("bytes inside the cap must be read")
	}
}

func TestObserve_StopsTwoFoldersDown(t *testing.T) {
	root := filepath.Join(t.TempDir(), "etc")
	write(t, filepath.Join(root, "a", "b", "file"), "1")
	spec := FileSpec{ID: "x", Paths: []string{root}}
	before := hashOf(t, spec).Hash
	write(t, filepath.Join(root, "a", "b", "other"), "2")
	if hashOf(t, spec).Hash != before {
		t.Error("a folder two levels down is named but not opened, so a bounded look cannot be sent through a deep tree")
	}
}
