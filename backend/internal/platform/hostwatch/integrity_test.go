package hostwatch

import (
	"context"
	"crypto/md5" // #nosec G501 -- dpkg's own checksum format; used to compare, never to protect
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tree is a fake root with a dpkg info directory: write puts a file under the root and the package's checksum line for it.
type tree struct {
	t    *testing.T
	root string
	info string
}

func newTree(t *testing.T) *tree {
	t.Helper()
	root := t.TempDir()
	info := filepath.Join(root, "var/lib/dpkg/info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	return &tree{t: t, root: root, info: info}
}

func sum(content string) string {
	s := md5.Sum([]byte(content)) // #nosec G401 -- see the import
	return hex.EncodeToString(s[:])
}

// write installs a file with content and records shipped as the checksum its package claims for it.
func (tr *tree) write(pkg, path, content, shipped string) {
	tr.t.Helper()
	full := filepath.Join(tr.root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	tr.claim(pkg, path, shipped)
}

func (tr *tree) claim(pkg, path, shipped string) {
	tr.t.Helper()
	f, err := os.OpenFile(filepath.Join(tr.info, pkg+".md5sums"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		tr.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(shipped + "  " + path + "\n"); err != nil {
		tr.t.Fatal(err)
	}
}

func (tr *tree) check() IntegrityResult {
	return Integrity{InfoDir: tr.info, Root: tr.root, Prefixes: DefaultIntegrityPrefixes}.Check(context.Background())
}

func TestIntegrity_AFileThatMatchesItsPackageIsNotReported(t *testing.T) {
	tr := newTree(t)
	tr.write("coreutils", "usr/bin/ls", "real ls", sum("real ls"))
	got := tr.check()
	if got.Modified != 0 || got.Checked != 1 || !got.Complete {
		t.Errorf("result = %+v", got)
	}
}

func TestIntegrity_AChangedProgramIsReportedWithItsPathAndAnUnchangedOneBesideItIsNot(t *testing.T) {
	tr := newTree(t)
	tr.write("openssh-server", "usr/sbin/sshd", "TROJAN", sum("original sshd"))
	tr.write("coreutils", "usr/bin/ls", "real ls", sum("real ls"))
	got := tr.check()
	if got.Modified != 1 || len(got.Paths) != 1 || got.Paths[0] != "/usr/sbin/sshd" || got.Checked != 2 {
		t.Errorf("result = %+v", got)
	}
}

func TestIntegrity_ConfigFilesAndPathsOutsideProgramDirectoriesAreNotLookedAt(t *testing.T) {
	tr := newTree(t)
	tr.write("openssh-server", "etc/ssh/sshd_config", "edited by the owner", sum("shipped"))
	tr.write("tool", "usr/lib/tool/settings", "edited", sum("shipped"))
	if err := os.WriteFile(filepath.Join(tr.info, "tool.conffiles"), []byte("/usr/lib/tool/settings\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr.write("docs", "usr/share/doc/x/README", "edited", sum("shipped"))
	if got := tr.check(); got.Modified != 0 || got.Checked != 0 {
		t.Errorf("result = %+v", got)
	}
}

func TestIntegrity_AMissingFileASymlinkAndAMalformedLineAreIgnoredNotReported(t *testing.T) {
	tr := newTree(t)
	tr.claim("gone", "usr/bin/removed", sum("x"))
	tr.write("real", "usr/bin/target", "t", sum("t"))
	if err := os.Symlink("target", filepath.Join(tr.root, "usr/bin/link")); err != nil {
		t.Fatal(err)
	}
	tr.claim("real", "usr/bin/link", sum("anything"))
	if err := os.WriteFile(filepath.Join(tr.info, "junk.md5sums"), []byte("not a checksum line\n\n../../etc/passwd  usr/bin/../../etc/passwd\nzz  usr/bin/ls\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tr.check(); got.Modified != 0 {
		t.Errorf("result = %+v", got)
	}
}

func TestIntegrity_AnUnreadableFileIsCountedAndNeverCalledUnchanged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	tr := newTree(t)
	tr.write("secret", "usr/bin/locked", "x", sum("x"))
	if err := os.Chmod(filepath.Join(tr.root, "usr/bin/locked"), 0); err != nil {
		t.Fatal(err)
	}
	got := tr.check()
	if got.Unreadable != 1 || got.Modified != 0 {
		t.Errorf("result = %+v", got)
	}
}

func TestIntegrity_UnreadableFilesAreNamedByDirectoryAndReason(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	tr := newTree(t)
	for _, f := range []string{"usr/lib/modules/a.ko", "usr/lib/modules/b.ko", "usr/lib/modules/sub/c.ko", "usr/bin/locked"} {
		tr.write("pkg", f, "x", sum("x"))
		if err := os.Chmod(filepath.Join(tr.root, f), 0); err != nil {
			t.Fatal(err)
		}
	}
	got := tr.check()
	want := []string{"/usr/lib/modules (3)", "/usr/bin (1)"}
	if !slices.Equal(got.UnreadableIn, want) {
		t.Errorf("UnreadableIn = %v, want %v", got.UnreadableIn, want)
	}
	if got.UnreadableWhy != "permission denied" {
		t.Errorf("UnreadableWhy = %q", got.UnreadableWhy)
	}
}

func TestIntegrity_TheDirectoriesNamedAreFewAndTheirCountsAddUpToWhatIsShown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	tr := newTree(t)
	for _, d := range []string{"a", "b", "c", "d", "e"} {
		f := "usr/lib/" + d + "/x"
		tr.write("pkg", f, "x", sum("x"))
		if err := os.Chmod(filepath.Join(tr.root, f), 0); err != nil {
			t.Fatal(err)
		}
	}
	if got := tr.check(); len(got.UnreadableIn) != MaxUnreadablePlaces || got.Unreadable != 5 {
		t.Errorf("UnreadableIn = %v, Unreadable = %d", got.UnreadableIn, got.Unreadable)
	}
}

func TestIntegrity_AFolderTheAccountCannotEnterCountsOnceNotOncePerFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	tr := newTree(t)
	for _, f := range []string{"usr/lib/modules/k1/a.ko", "usr/lib/modules/k1/b.ko", "usr/lib/modules/k1/sub/c.ko"} {
		tr.write("pkg", f, "x", sum("x"))
	}
	tr.write("pkg", "usr/bin/locked", "x", sum("x"))
	if err := os.Chmod(filepath.Join(tr.root, "usr/lib/modules/k1"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(tr.root, "usr/lib/modules/k1"), 0o755) })
	got := tr.check()
	if got.Unreadable != 1 || !slices.Equal(got.UnreadableIn, []string{"/usr/lib/modules/k1 (1)"}) {
		t.Errorf("Unreadable = %d, in = %v; want the folder counted once", got.Unreadable, got.UnreadableIn)
	}
}

func TestIntegrity_APassThatIsCutShortSaysSoAndNeverAnAllClear(t *testing.T) {
	tr := newTree(t)
	for i := 0; i < 20; i++ {
		tr.write("pkg", "usr/bin/p"+string(rune('a'+i)), "c", sum("c"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := Integrity{InfoDir: tr.info, Root: tr.root, Prefixes: DefaultIntegrityPrefixes}.Check(ctx)
	if got.Complete {
		t.Errorf("a cancelled pass is not complete: %+v", got)
	}
}

func TestIntegrity_AFileChangedByAPackageUpdateDuringThePassIsNotReported(t *testing.T) {
	tr := newTree(t)
	tr.write("openssh-server", "usr/sbin/sshd", "new version", sum("old version"))
	i := Integrity{InfoDir: tr.info, Root: tr.root, Prefixes: DefaultIntegrityPrefixes}
	// The update finishes after the first look: the package's checksum list now matches the new file.
	i.afterPass = func() {
		if err := os.WriteFile(filepath.Join(tr.info, "openssh-server.md5sums"), []byte(sum("new version")+"  usr/sbin/sshd\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := i.Check(context.Background()); got.Modified != 0 {
		t.Errorf("an update in progress was reported as tampering: %+v", got)
	}
}

func TestIntegrity_OnlyAPlainSafePathIsListedAndTheListIsBounded(t *testing.T) {
	tr := newTree(t)
	for i := 0; i < 25; i++ {
		tr.write("pkg", "usr/bin/bad"+strings.Repeat("x", i), "changed", sum("shipped"))
	}
	// It sorts before every other name, so the cap cannot hide it: only the filter keeps it out of the list.
	tr.write("odd", "usr/bin/Aodd name;rm", "changed", sum("shipped"))
	got := tr.check()
	if got.Modified != 26 || len(got.Paths) != MaxIntegrityPaths {
		t.Fatalf("modified %d, listed %d", got.Modified, len(got.Paths))
	}
	for _, p := range got.Paths {
		if strings.ContainsAny(p, " ;") {
			t.Errorf("an unsafe path was listed: %q", p)
		}
	}
}

func TestIntegrity_ADirectoryThatCannotBeReadIsNotACleanPass(t *testing.T) {
	got := Integrity{InfoDir: filepath.Join(t.TempDir(), "missing"), Root: t.TempDir(), Prefixes: DefaultIntegrityPrefixes}.Check(context.Background())
	if got.Complete || got.Checked != 0 {
		t.Errorf("result = %+v", got)
	}
}
