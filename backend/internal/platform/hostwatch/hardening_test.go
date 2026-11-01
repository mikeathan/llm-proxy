package hostwatch

import (
	"os"
	"path/filepath"
	"testing"
)

// sshRoot builds an /etc/ssh look-alike: the real Ubuntu file as sshd_config and the given drop-in files.
func sshRoot(t *testing.T, main string, dropIns map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ssh")
	if err := os.MkdirAll(filepath.Join(dir, "sshd_config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sshd_config"), main)
	for name, body := range dropIns {
		write(t, filepath.Join(dir, "sshd_config.d", name), body)
	}
	return dir
}

func realMain(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/sshd_config_ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ssh(t *testing.T, main string, dropIns map[string]string) Hardening {
	t.Helper()
	return Hardener{SSHDir: sshRoot(t, main, dropIns)}.Check()
}

func TestHardening_UbuntuDefaultsAreOnlyPasswordLogin(t *testing.T) {
	// The stock file on the server: every option is commented out except a few, so sshd's own defaults apply:
	// passwords allowed, root login only with a key, no empty passwords.
	h := ssh(t, realMain(t), nil)
	if !h.SSHFound || !h.PasswordLogin || h.RootLogin || h.EmptyPasswords {
		t.Errorf("got %+v", h)
	}
}

func TestHardening_ADropInBeforeTheRestWins(t *testing.T) {
	// The stock Include is at the top, so a drop-in's value is the first one sshd sees and wins over the main file's.
	h := ssh(t, realMain(t)+"\nPasswordAuthentication yes\n", map[string]string{"50-cloud.conf": "PasswordAuthentication no\n"})
	if h.PasswordLogin {
		t.Error("the first value wins: the drop-in said no")
	}
}

func TestHardening_FirstValueWinsWithinAFileAndKeywordsIgnoreCase(t *testing.T) {
	h := ssh(t, "permitrootlogin NO\nPermitRootLogin yes\nPasswordAuthentication=no\nPermitEmptyPasswords yes\n", nil)
	if h.RootLogin || h.PasswordLogin || !h.EmptyPasswords {
		t.Errorf("got %+v", h)
	}
}

func TestHardening_RootLoginOnlyWhenPlainYes(t *testing.T) {
	for value, want := range map[string]bool{"yes": true, "prohibit-password": false, "without-password": false, "forced-commands-only": false, "no": false} {
		if got := ssh(t, "PermitRootLogin "+value+"\n", nil).RootLogin; got != want {
			t.Errorf("PermitRootLogin %s = %v, want %v", value, got, want)
		}
	}
}

func TestHardening_AMatchBlockIsConditionalAndNotTheDefault(t *testing.T) {
	h := ssh(t, "PasswordAuthentication no\nMatch User guest\n  PermitRootLogin yes\n  PermitEmptyPasswords yes\n", nil)
	if h.RootLogin || h.EmptyPasswords {
		t.Errorf("settings after Match apply to some connections only: %+v", h)
	}
}

func TestHardening_AnIncludeCannotLeaveTheSSHFolder(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "evil.conf")
	write(t, outside, "PermitRootLogin yes\n")
	h := ssh(t, "Include "+outside+"\nInclude ../../../../"+outside+"\n", nil)
	if h.RootLogin {
		t.Error("a config file must not send the reader to files outside /etc/ssh")
	}
}

func TestHardening_NoSSHServerIsNotAFinding(t *testing.T) {
	h := Hardener{SSHDir: filepath.Join(t.TempDir(), "absent")}.Check()
	if h.SSHFound || h.PasswordLogin || h.Unreadable != 0 {
		t.Errorf("a computer with no SSH server has nothing to report: %+v", h)
	}
}

func TestHardening_ReverseFilterOffOnlyWhenBothAreZero(t *testing.T) {
	sys := t.TempDir()
	set := func(all, def string) Hardening {
		write(t, filepath.Join(sys, "net", "ipv4", "conf", "all", "rp_filter"), all+"\n")
		write(t, filepath.Join(sys, "net", "ipv4", "conf", "default", "rp_filter"), def+"\n")
		return Hardener{SysDir: sys}.Check()
	}
	// 2 is "loose" and Ubuntu's own default (the server reads 2 and 2); 1 is strict.
	for _, c := range []struct {
		all, def string
		off      bool
	}{{"2", "2", false}, {"1", "1", false}, {"0", "2", false}, {"2", "0", false}, {"0", "0", true}} {
		if got := set(c.all, c.def).RPFilterOff; got != c.off {
			t.Errorf("all=%s default=%s: RPFilterOff = %v, want %v", c.all, c.def, got, c.off)
		}
	}
	if (Hardener{SysDir: t.TempDir()}).Check().RPFilterOff {
		t.Error("a value that cannot be read is not 'off'")
	}
}

func TestHardening_CountsAnUnreadableConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	dir := sshRoot(t, "PasswordAuthentication no\n", nil)
	if err := os.Chmod(filepath.Join(dir, "sshd_config"), 0); err != nil {
		t.Fatal(err)
	}
	if h := (Hardener{SSHDir: dir}).Check(); h.Unreadable != 1 || h.PasswordLogin {
		t.Errorf("an unreadable file is counted and reports nothing: %+v", h)
	}
}
