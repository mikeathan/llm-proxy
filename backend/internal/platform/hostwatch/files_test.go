package hostwatch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

// snapd writes a snap-<name>-<rev>.mount unit (and enables it with a link in multi-user.target.wants) at every snap refresh.
// Those are routine, so the watch skips them by file name; anything else a person or a program adds must still be caught.
func TestObserve_SnapMountUnitsAreSkippedEverythingElseIsCaught(t *testing.T) {
	dir := t.TempDir()
	units := filepath.Join(dir, "system")
	write(t, filepath.Join(units, "app.service"), "[Service]\n")
	write(t, filepath.Join(units, "snap-core22-2955.mount"), "[Mount]\nWhat=/var/lib/snapd/snaps/core22_2955.snap\n")
	wants := filepath.Join(units, "multi-user.target.wants")
	if err := os.MkdirAll(wants, 0o755); err != nil {
		t.Fatal(err)
	}
	link := func(name string) {
		t.Helper()
		if err := os.Symlink(filepath.Join(units, name), filepath.Join(wants, name)); err != nil {
			t.Fatal(err)
		}
	}
	link("snap-core22-2955.mount")
	spec := FileSpec{ID: "systemd_units", Paths: []string{units}, SkipNames: []string{"snap-*.mount"}}
	before := hashOf(t, spec).Hash

	// A snap refresh: new revisions arrive, old ones go, and the text of a unit may differ.
	for _, name := range []string{"snap-core22-2987.mount", "snap-firefox-9036.mount", "snap-firefox-9057.mount"} {
		write(t, filepath.Join(units, name), "[Mount]\nWhat=/var/lib/snapd/snaps/"+name+"\n")
		link(name)
	}
	if err := os.Remove(filepath.Join(units, "snap-core22-2955.mount")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(wants, "snap-core22-2955.mount")); err != nil {
		t.Fatal(err)
	}
	if got := hashOf(t, spec).Hash; got != before {
		t.Error("snapd's mount units and their links must not change the hash")
	}

	for name, path := range map[string]string{
		"a service named like a snap": filepath.Join(units, "snap-evil.service"),
		"a timer":                     filepath.Join(units, "snap-core22.timer"),
		"a mount with another name":   filepath.Join(units, "mnt-data.mount"),
		"a drop-in for a snap mount":  filepath.Join(units, "snap-core22-2955.mount.d", "override.conf"),
	} {
		write(t, path, "[Service]\n")
		now := hashOf(t, spec).Hash
		if now == before {
			t.Errorf("%s must change the hash", name)
		}
		os.RemoveAll(path)
		os.Remove(filepath.Dir(path)) // an emptied drop-in folder, so it cannot colour the next case
	}
}

func TestSource_IntegrityStopsWhenTheCallersContextEnds(t *testing.T) {
	tr := newTree(t)
	tr.write("coreutils", "usr/bin/ls", "ls", sum("ls"))
	src := Source{Packages: Integrity{InfoDir: tr.info, Root: tr.root, Prefixes: DefaultIntegrityPrefixes}, PackagesBudget: time.Hour}

	if got := src.Integrity(t.Context()); !got.Complete || got.Checked != 1 {
		t.Fatalf("a live context: got %+v, want a complete pass over one file", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the run or the application stopped
	if got := src.Integrity(ctx); got.Complete {
		t.Fatalf("a pass whose caller has stopped must say it did not finish, got %+v", got)
	}
}

func idsOf(specs []FileSpec) []string {
	var ids []string
	for _, s := range specs {
		ids = append(ids, s.ID)
	}
	return ids
}

// Linux watches exactly the default places; macOS adds where a program is started at boot and at login, and the SSH keys under /Users.
func TestFileSpecsFor_LinuxIsTheDefaultAndMacOSAddsLaunchdAndItsHomes(t *testing.T) {
	linux := FileSpecsFor("linux", "/home/svc", "/usr/bin/llm-proxy")
	if !slices.Equal(idsOf(linux), idsOf(DefaultFileSpecs("/home/svc", "/usr/bin/llm-proxy"))) {
		t.Errorf("linux must be unchanged: %v", idsOf(linux))
	}
	mac := FileSpecsFor("darwin", "/Users/me", "/usr/local/bin/llm-proxy")
	byID := map[string]FileSpec{}
	for _, s := range mac {
		byID[s.ID] = s
	}
	if !slices.Equal(byID["launch_daemons"].Paths, []string{"/Library/LaunchDaemons"}) {
		t.Errorf("launch_daemons: %+v", byID["launch_daemons"])
	}
	if !slices.Equal(byID["launch_agents"].Paths, []string{"/Library/LaunchAgents", "/Users/me/Library/LaunchAgents"}) {
		t.Errorf("launch_agents: %+v", byID["launch_agents"])
	}
	if !slices.Contains(byID["authorized_keys"].Globs, "/Users/*/.ssh/authorized_keys") || !slices.Contains(byID["authorized_keys"].Globs, "/home/*/.ssh/authorized_keys") {
		t.Errorf("authorized_keys globs: %v", byID["authorized_keys"].Globs)
	}
	if slices.Contains(idsOf(linux), "launch_daemons") {
		t.Error("launchd places do not exist on Linux")
	}
	// An unknown home leaves its own folder out and nothing else.
	if noHome := FileSpecsFor("darwin", "", ""); !slices.Equal(noHome[len(noHome)-1].Paths, []string{"/Library/LaunchAgents"}) {
		t.Errorf("no home: %+v", noHome[len(noHome)-1])
	}
}
