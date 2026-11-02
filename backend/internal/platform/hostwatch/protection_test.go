package hostwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// protect builds an /etc/apt/apt.conf.d and an /etc/ufw/ufw.conf look-alike and reads them.
func protect(t *testing.T, apt map[string]string, ufw *string) Hardening {
	t.Helper()
	root := t.TempDir()
	h := Hardener{}
	if apt != nil {
		h.AptDir = filepath.Join(root, "apt.conf.d")
		if err := os.MkdirAll(h.AptDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range apt {
			write(t, filepath.Join(h.AptDir, name), body)
		}
	}
	if ufw != nil {
		h.UfwConf = filepath.Join(root, "ufw.conf")
		write(t, h.UfwConf, *ufw)
	}
	return h.Check()
}

func TestProtection_TheCapturedVertexFilesAreAutoUpdatesOnAndFirewallOff(t *testing.T) {
	ufw := fixture(t, "ufw_conf_ubuntu_disabled")
	h := protect(t, map[string]string{"20auto-upgrades": fixture(t, "20auto-upgrades_ubuntu")}, &ufw)
	if h.AutoUpdatesOff || !h.FirewallOff {
		t.Errorf("got %+v", h)
	}
}

func TestProtection_AutoUpdatesAreOffWhenZeroOrNotSetOnAnAptComputer(t *testing.T) {
	cases := map[string]map[string]string{
		"zero":    {"20auto-upgrades": "APT::Periodic::Unattended-Upgrade \"0\";\n"},
		"unset":   {"20auto-upgrades": "APT::Periodic::Update-Package-Lists \"1\";\n"},
		"no file": {"50other": "Acquire::Languages \"none\";\n"},
	}
	for name, apt := range cases {
		if h := protect(t, apt, nil); !h.AutoUpdatesOff {
			t.Errorf("%s: got %+v", name, h)
		}
	}
}

func TestProtection_ALaterFileOverridesAnEarlierOneAndCommentsAreIgnored(t *testing.T) {
	on := protect(t, map[string]string{
		"10periodic":      "APT::Periodic::Unattended-Upgrade \"0\";\n",
		"20auto-upgrades": "// APT::Periodic::Unattended-Upgrade \"0\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n",
	}, nil)
	if on.AutoUpdatesOff {
		t.Errorf("later file said 1: %+v", on)
	}
	off := protect(t, map[string]string{
		"20auto-upgrades": "APT::Periodic::Unattended-Upgrade \"1\";\n",
		"99local":         "APT::Periodic::Unattended-Upgrade \"0\";\n",
	}, nil)
	if !off.AutoUpdatesOff {
		t.Errorf("later file said 0: %+v", off)
	}
}

func TestProtection_ADaysValueMeansOn(t *testing.T) {
	if h := protect(t, map[string]string{"20auto-upgrades": "APT::Periodic::Unattended-Upgrade \"7\";\n"}, nil); h.AutoUpdatesOff {
		t.Errorf("7 days is on: %+v", h)
	}
}

func TestProtection_NotAnAptComputerOrUnreadableSaysNothing(t *testing.T) {
	if h := protect(t, nil, nil); h.AutoUpdatesOff || h.FirewallOff {
		t.Errorf("nothing configured: %+v", h)
	}
	h := Hardener{AptDir: filepath.Join(t.TempDir(), "missing"), UfwConf: filepath.Join(t.TempDir(), "missing.conf")}.Check()
	if h.AutoUpdatesOff || h.FirewallOff || h.Unreadable != 0 {
		t.Errorf("missing files are not a finding and not unreadable: %+v", h)
	}
}

func TestProtection_FirewallIsOffOnlyWhenEnabledIsNo(t *testing.T) {
	cases := map[string]bool{
		"ENABLED=no\n":              true,
		"enabled = \"NO\"\n":        true,
		"ENABLED=yes\n":             false,
		"# ENABLED=no\n":            false,
		"LOGLEVEL=low\n":            false,
		"ENABLED=no\nENABLED=yes\n": false,
	}
	for body, want := range cases {
		body := body
		if got := protect(t, nil, &body).FirewallOff; got != want {
			t.Errorf("%q: FirewallOff = %v, want %v", strings.TrimSpace(body), got, want)
		}
	}
}
