package hostwatch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// privilegeFixture copies the Ubuntu-shaped fixtures into a temp folder and returns a reader over them; extra lines can be
// appended to each file before the read.
func privilegeFixture(t *testing.T, passwdExtra, groupExtra string) Privileger {
	t.Helper()
	dir := t.TempDir()
	copyWith := func(name, extra string) string {
		b, err := os.ReadFile(filepath.Join("testdata", "privilege", name))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, append(b, extra...), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return Privileger{
		Passwd:  copyWith("passwd_ubuntu", passwdExtra),
		Group:   copyWith("group_ubuntu", groupExtra),
		Modules: copyWith("modules_ubuntu", ""),
		Preload: filepath.Join(dir, "ld.so.preload"),
	}
}

func TestPrivileger_AStockUbuntuReadsCleanly(t *testing.T) {
	got := privilegeFixture(t, "", "").Check()
	if !got.Checked || got.Preload || len(got.ExtraRoot) != 0 || got.Unreadable != 0 {
		t.Fatalf("stock system = %+v", got)
	}
	if want := []string{"owner", "root"}; !slices.Equal(got.LoginAccounts, want) {
		t.Errorf("login accounts = %v, want %v (nologin, false and sync are not logins)", got.LoginAccounts, want)
	}
	if want := []string{"adm:owner", "adm:syslog", "lxd:owner", "sudo:owner"}; !slices.Equal(got.Admins, want) {
		t.Errorf("admins = %v, want %v", got.Admins, want)
	}
	if want := []string{"iwlwifi", "nf_nat", "nft_chain_nat", "overlay"}; !slices.Equal(got.Modules, want) || !got.ModulesRead {
		t.Errorf("modules = %v (read %v), want %v", got.Modules, got.ModulesRead, want)
	}
}

func TestPrivileger_ASecondRootAccountAndAPrimaryAdminGroupAreSeen(t *testing.T) {
	got := privilegeFixture(t, "toor:x:0:0::/root:/bin/sh\nhelper:x:1001:27::/home/helper:/bin/bash\n", "").Check()
	if !slices.Equal(got.ExtraRoot, []string{"toor"}) {
		t.Errorf("extra root = %v, want [toor]", got.ExtraRoot)
	}
	if !slices.Contains(got.Admins, "sudo:helper") {
		t.Errorf("an account whose own group is sudo is an administrator: %v", got.Admins)
	}
}

func TestPrivileger_APreloadFileIsSeenOnlyWhenItListsSomething(t *testing.T) {
	r := privilegeFixture(t, "", "")
	if err := os.WriteFile(r.Preload, []byte("# empty\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.Check(); got.Preload {
		t.Fatal("a preload file of comments only loads nothing")
	}
	if err := os.WriteFile(r.Preload, []byte("/usr/lib/libevil.so\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.Check(); !got.Preload || got.PreloadDigest == "" {
		t.Fatalf("a preload file that names a library must be seen: %+v", got)
	}
}

func TestPrivileger_NamesThatAreNotPlainTokensAreLeftOutAndCounted(t *testing.T) {
	got := privilegeFixture(t, "bad name;rm:x:1002:1002::/home/x:/bin/bash\n", "").Check()
	if slices.ContainsFunc(got.LoginAccounts, func(s string) bool { return s == "bad name;rm" }) {
		t.Fatal("a name with spaces or punctuation must never be kept")
	}
	if got.Odd != 1 {
		t.Errorf("odd = %d, want 1", got.Odd)
	}
}

func TestPrivileger_AComputerWithoutTheFilesIsNotChecked(t *testing.T) {
	dir := t.TempDir()
	got := Privileger{Passwd: filepath.Join(dir, "none"), Group: filepath.Join(dir, "none"), Modules: filepath.Join(dir, "none")}.Check()
	if got.Checked {
		t.Fatalf("no account file means not checked, never fine: %+v", got)
	}
}

func TestPrivileger_SetIDFilesUnderTheRootsAreListed(t *testing.T) {
	r := privilegeFixture(t, "", "")
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, mode := range map[string]os.FileMode{"plain": 0o755, "suid": 0o755 | os.ModeSetuid, "sgid": 0o755 | os.ModeSetgid} {
		p := filepath.Join(bin, name)
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	r.SetIDRoots = []string{root}
	got := r.Check()
	want := []string{filepath.Join(bin, "sgid"), filepath.Join(bin, "suid")}
	if !slices.Equal(got.SetID, want) || !got.SetIDComplete {
		t.Fatalf("set-id files = %v (complete %v), want %v", got.SetID, got.SetIDComplete, want)
	}
	r.MaxSetIDEntries = 1
	if got := r.Check(); got.SetIDComplete {
		t.Fatal("a walk cut short by its limit must say it is incomplete")
	}
}

func TestPrivileger_AClosedSubfolderIsCountedAndTheWalkStaysComplete(t *testing.T) {
	r := privilegeFixture(t, "", "")
	root := t.TempDir()
	closed := filepath.Join(root, "containerd")
	if err := os.MkdirAll(filepath.Join(closed, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o755) })
	r.SetIDRoots = []string{root, filepath.Join(root, "missing")}
	got := r.Check()
	if !got.SetIDComplete || got.Unreadable != 1 {
		t.Fatalf("complete = %v, unreadable = %d; a closed folder below a root is one unreadable place, not an unfinished walk", got.SetIDComplete, got.Unreadable)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if got := r.Check(); got.SetIDComplete {
		t.Fatal("a root that cannot be read leaves the walk incomplete")
	}
}
