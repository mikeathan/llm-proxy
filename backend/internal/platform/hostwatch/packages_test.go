package hostwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dpkgStatus = `Package: openssl
Status: install ok installed
Priority: optional
Section: utils
Architecture: amd64
Version: 3.0.13-0ubuntu3
Description: Secure Sockets Layer toolkit - cryptographic utility
 Package: not-a-real-stanza
 Status: install ok installed
 Version: 99

Package: libssl3t64
Status: install ok installed
Architecture: amd64
Multi-Arch: same
Version: 3.0.13-0ubuntu3

Package: libssl3t64
Status: install ok installed
Architecture: i386
Multi-Arch: same
Version: 3.0.13-0ubuntu3

Package: removed-but-configured
Status: deinstall ok config-files
Version: 1.0-1

Package: half
Status: install ok half-installed
Version: 2.0-1

Package: held-package
Status: hold ok installed
Version: 4:5.1-2ubuntu1.1

Package: Bad Name;rm
Status: install ok installed
Version: 1.0

Package: bad-version
Status: install ok installed
Version: $(rm -rf /)

Package: no-version
Status: install ok installed

Package: curl
Status: install ok installed
Version: 8.5.0-2ubuntu10.5
`

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPackages_ListsOnlyWhatIsInstalledWithAPlainNameAndVersion(t *testing.T) {
	got, err := Packages{StatusFile: writeFile(t, dpkgStatus)}.Installed()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"openssl": "3.0.13-0ubuntu3", "libssl3t64": "3.0.13-0ubuntu3", "held-package": "4:5.1-2ubuntu1.1", "curl": "8.5.0-2ubuntu10.5"}
	if len(got) != len(want) {
		t.Fatalf("installed = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestPackages_ALineInsideADescriptionIsNeverAStanza(t *testing.T) {
	got, _ := Packages{StatusFile: writeFile(t, dpkgStatus)}.Installed()
	if _, ok := got["not-a-real-stanza"]; ok {
		t.Error("a continuation line was read as a package")
	}
}

func TestPackages_AFileThatCannotBeReadIsAnErrorNeverAnEmptyList(t *testing.T) {
	if got, err := (Packages{StatusFile: filepath.Join(t.TempDir(), "missing")}).Installed(); err == nil || got != nil {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := (Packages{}).Installed(); err == nil {
		t.Error("no file configured is an error")
	}
	if _, err := (Packages{StatusFile: writeFile(t, "")}).Installed(); err == nil {
		t.Error("a file with no package is not a package list")
	}
}

func TestPackages_IsBoundedInSizeAndCount(t *testing.T) {
	var b strings.Builder
	for i := 0; i < MaxPackages+50; i++ {
		b.WriteString("Package: p")
		b.WriteString(strings.Repeat("a", 3))
		b.WriteString(itoa(i))
		b.WriteString("\nStatus: install ok installed\nVersion: 1.0\n\n")
	}
	got, err := Packages{StatusFile: writeFile(t, b.String())}.Installed()
	if err != nil || len(got) > MaxPackages {
		t.Errorf("listed %d packages, err %v", len(got), err)
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var out []byte
	for ; i > 0; i /= 10 {
		out = append([]byte{digits[i%10]}, out...)
	}
	return string(out)
}

func TestPackages_ReleaseIsTheCodenameOfUbuntuAndNothingForAnythingElse(t *testing.T) {
	cases := map[string]string{
		"ID=ubuntu\nVERSION_CODENAME=noble\n":                         "noble",
		"NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_CODENAME=\"jammy\"\n":    "jammy",
		"ID=debian\nVERSION_CODENAME=bookworm\n":                      "",
		"ID=ubuntu\n":                                                 "",
		"ID=ubuntu\nVERSION_CODENAME=../etc\n":                        "",
		"ID=pop\nID_LIKE=\"ubuntu debian\"\nVERSION_CODENAME=jammy\n": "",
		"VERSION_CODENAME=noble\n":                                    "",
	}
	for body, want := range cases {
		if got := (Packages{OSRelease: writeFile(t, body)}).Release(); got != want {
			t.Errorf("%q: release %q, want %q", body, got, want)
		}
	}
	if got := (Packages{OSRelease: filepath.Join(t.TempDir(), "missing")}).Release(); got != "" {
		t.Errorf("a missing file names no release, got %q", got)
	}
}

func TestPackages_AnArchitectureCopyDoesNotReplaceTheFirstListedVersion(t *testing.T) {
	body := "Package: libfoo\nStatus: install ok installed\nVersion: 1.0-1\n\nPackage: libfoo\nStatus: install ok installed\nVersion: 2.0-1\n\nPackage: other\nStatus: install ok installed\nVersion: 1\n"
	got, err := Packages{StatusFile: writeFile(t, body)}.Installed()
	if err != nil || got["libfoo"] != "1.0-1" {
		t.Errorf("libfoo = %q, err %v", got["libfoo"], err)
	}
}

func TestPackages_ANewPackageLineEndsTheEntryBeforeEvenWithoutABlankLine(t *testing.T) {
	body := "Package: first\nStatus: install ok installed\nVersion: 1.0\nPackage: second\nStatus: install ok installed\nVersion: 2.0\n"
	got, err := Packages{StatusFile: writeFile(t, body)}.Installed()
	if err != nil || got["first"] != "1.0" || got["second"] != "2.0" {
		t.Errorf("packages = %v, err %v", got, err)
	}
}
