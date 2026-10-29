package boot

import (
	"bytes"
	"errors"
	"llm-proxy/internal/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit setup.sh installs (docs/services/llm-proxy.service), trimmed.
const unitText = `[Unit]
Description=LLM Proxy Service

[Service]
# Dedicated unprivileged account.
User=llm-proxy
Group=llm-proxy
Environment=APP_ENV=production
# Single root.
Environment=LLM_PROXY_HOME=/var/lib/llm-proxy
WorkingDirectory=/var/lib/llm-proxy
ExecStart=/usr/local/bin/llm-proxy
Restart=on-failure
`

func fakeHome(user string) (string, error) { return "/home/" + user, nil }

func TestParseUnit(t *testing.T) {
	svc, ok := parseUnit(unitText, fakeHome)
	if !ok || svc.User != "llm-proxy" || svc.Home != "/var/lib/llm-proxy" || svc.Binary != "/usr/local/bin/llm-proxy" {
		t.Fatalf("%+v %v", svc, ok)
	}
}

func TestParseUnit_DataFlagWinsThenTheEnvironmentThenTheUsersConfigDir(t *testing.T) {
	flagged := strings.Replace(unitText, "ExecStart=/usr/local/bin/llm-proxy", "ExecStart=/usr/local/bin/llm-proxy --data /srv/llm", 1)
	if svc, _ := parseUnit(flagged, fakeHome); svc.Home != "/srv/llm" {
		t.Errorf("--data in ExecStart = %q, want /srv/llm", svc.Home)
	}
	equals := strings.Replace(unitText, "ExecStart=/usr/local/bin/llm-proxy", "ExecStart=/usr/local/bin/llm-proxy --data=/srv/eq", 1)
	if svc, _ := parseUnit(equals, fakeHome); svc.Home != "/srv/eq" {
		t.Errorf("--data=… in ExecStart = %q", svc.Home)
	}
	quoted := strings.Replace(unitText, "Environment=LLM_PROXY_HOME=/var/lib/llm-proxy", `Environment="A=1" "LLM_PROXY_HOME=/quoted/dir"`, 1)
	if svc, _ := parseUnit(quoted, fakeHome); svc.Home != "/quoted/dir" {
		t.Errorf("quoted environment = %q", svc.Home)
	}
	noEnv := strings.Replace(unitText, "Environment=LLM_PROXY_HOME=/var/lib/llm-proxy\n", "", 1)
	if svc, _ := parseUnit(noEnv, fakeHome); svc.Home != "/home/llm-proxy/.config/llm-proxy" {
		t.Errorf("no setting = %q, want the user's default config dir", svc.Home)
	}
	root := strings.Replace(noEnv, "User=llm-proxy\n", "", 1)
	if svc, _ := parseUnit(root, fakeHome); svc.User != "root" {
		t.Errorf("a service with no User runs as root: %+v", svc)
	}
	prefixed := strings.Replace(unitText, "ExecStart=/usr/local/bin/llm-proxy", "ExecStart=-/usr/local/bin/llm-proxy", 1)
	if svc, _ := parseUnit(prefixed, fakeHome); svc.Binary != "/usr/local/bin/llm-proxy" {
		t.Errorf("a systemd prefix on the path: %q", svc.Binary)
	}
}

func TestParseUnit_NotUsable(t *testing.T) {
	for _, text := range []string{"", "garbage", "[Service]\nUser=x\n"} {
		if _, ok := parseUnit(text, fakeHome); ok {
			t.Errorf("%q has no ExecStart: it cannot describe the service", text)
		}
	}
}

func TestLocateUnit(t *testing.T) {
	read := func(want string) func(string) ([]byte, error) {
		return func(path string) ([]byte, error) {
			if path == want {
				return []byte(unitText), nil
			}
			return nil, os.ErrNotExist
		}
	}
	if svc, ok := locateUnit(read("/etc/systemd/system/llm-proxy.service"), fakeHome); !ok || svc.User != "llm-proxy" {
		t.Errorf("the unit setup.sh installs: %+v %v", svc, ok)
	}
	if _, ok := locateUnit(read("/usr/lib/systemd/system/llm-proxy.service"), fakeHome); !ok {
		t.Error("a vendor-installed unit")
	}
	if _, ok := locateUnit(read("/nowhere"), fakeHome); ok {
		t.Error("no unit file means no installed service to find")
	}
}

// fakes replaces everything the command asks the machine, and restores it afterwards.
type fakes struct {
	unit      *installedService
	euid      int
	owner     map[string][2]int // folder -> owning uid, gid (a missing entry is a folder that does not exist yet)
	ids       map[string][2]int // account name -> uid, gid
	became    [][2]int
	becomeErr error
}

func installFakes(t *testing.T, f *fakes) {
	t.Helper()
	l, b, u, o, i := locateService, becomeUser, currentUID, ownerOf, idsOf
	t.Cleanup(func() { locateService, becomeUser, currentUID, ownerOf, idsOf = l, b, u, o, i })
	locateService = func() (installedService, bool) {
		if f.unit == nil {
			return installedService{}, false
		}
		return *f.unit, true
	}
	becomeUser = func(uid, gid int) error { f.became = append(f.became, [2]int{uid, gid}); return f.becomeErr }
	currentUID = func() int { return f.euid }
	ownerOf = func(path string) (int, int, bool) { v, ok := f.owner[path]; return v[0], v[1], ok }
	idsOf = func(name string) (int, int, bool) { v, ok := f.ids[name]; return v[0], v[1], ok }
}

func runCLI(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := RunAuthCLI(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunAuthCLI_ADeveloperActsOnTheirOwnFolder_AndIsToldAboutTheService(t *testing.T) {
	// `go run . auth setup-code`: no folder named, a service installed elsewhere. Debugging on the server must keep working.
	home, svcHome := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LLM_PROXY_HOME", "")
	f := &fakes{euid: 1000, unit: &installedService{User: "llm-proxy", Home: svcHome, Binary: "/usr/local/bin/llm-proxy"}}
	installFakes(t, f)

	code, out, errOut := runCLI("setup-code")
	own := filepath.Join(home, ".config", "llm-proxy", "auth.json")
	if code != 0 || !strings.Contains(out, own) {
		t.Fatalf("= %d, stdout %q: a developer acts on their own folder", code, out)
	}
	if _, err := os.Stat(own); err != nil {
		t.Errorf("nothing was written to their folder: %v", err)
	}
	if !strings.Contains(errOut, "no running server") || !strings.Contains(errOut, svcHome) || !strings.Contains(errOut, "sudo /usr/local/bin/llm-proxy auth setup-code") {
		t.Errorf("stderr %q: want the warning, the service's folder and the exact command", errOut)
	}
	if len(f.became) != 0 {
		t.Errorf("a non-root process cannot switch account: %v", f.became)
	}
}

func TestRunAuthCLI_RootWithAServiceAndNoFolderNamed_ActsAsTheFolderOwner(t *testing.T) {
	svcHome := t.TempDir()
	t.Setenv("LLM_PROXY_HOME", "")
	f := &fakes{euid: 0, unit: &installedService{User: "llm-proxy", Home: svcHome, Binary: "/usr/local/bin/llm-proxy"}, owner: map[string][2]int{svcHome: {998, 997}}}
	installFakes(t, f)
	code, out, _ := runCLI("setup-code")
	if code != 0 || !strings.Contains(out, filepath.Join(svcHome, "auth.json")) {
		t.Fatalf("= %d %q: root must use the service's folder", code, out)
	}
	if len(f.became) != 1 || f.became[0] != [2]int{998, 997} {
		t.Errorf("became %v, want the folder's owner 998:997", f.became)
	}
}

func TestRunAuthCLI_TheAccountIsTheFolderOwner_SoPlainSudoWithDataIsSafe(t *testing.T) {
	// `sudo llm-proxy auth setup-code --data /var/lib/llm-proxy` must not leave a root-owned 0600 file the service cannot read.
	dir := t.TempDir()
	f := &fakes{euid: 0, owner: map[string][2]int{dir: {998, 997}}}
	installFakes(t, f)
	if code, _, errOut := runCLI("setup-code", "--data", dir); code != 0 {
		t.Fatalf("= %d %q", code, errOut)
	}
	if len(f.became) != 1 || f.became[0] != [2]int{998, 997} {
		t.Errorf("became %v, want 998:997", f.became)
	}
	f.became = nil
	root := t.TempDir()
	f.owner[root] = [2]int{0, 0}
	if code, _, _ := runCLI("status", "--data", root); code != 0 || len(f.became) != 0 {
		t.Errorf("a root-owned folder is acted on as root: %d %v", code, f.became)
	}
}

func TestRunAuthCLI_ANewFolderOfTheServiceIsCreatedAsTheServicesAccount(t *testing.T) {
	svcHome := filepath.Join(t.TempDir(), "llm")
	t.Setenv("LLM_PROXY_HOME", "")
	f := &fakes{euid: 0, unit: &installedService{User: "llm-proxy", Home: svcHome, Binary: "/b"}, ids: map[string][2]int{"llm-proxy": {998, 997}}}
	installFakes(t, f)
	if code, _, errOut := runCLI("status"); code != 0 {
		t.Fatalf("= %d %q", code, errOut)
	}
	if len(f.became) != 1 || f.became[0] != [2]int{998, 997} {
		t.Errorf("a folder that does not exist yet is created by the service's account: %v", f.became)
	}
}

func TestRunAuthCLI_ANamedFolderIsBelieved_EvenFromTheEnvironment(t *testing.T) {
	// LLM_PROXY_HOME may come from a .env file: it is loaded before the folder is chosen, exactly as the server does.
	named, svcHome := t.TempDir(), t.TempDir()
	t.Setenv("LLM_PROXY_HOME", named)
	f := &fakes{euid: 0, unit: &installedService{User: "llm-proxy", Home: svcHome, Binary: "/b"}, owner: map[string][2]int{named: {0, 0}}}
	installFakes(t, f)
	code, out, errOut := runCLI("status")
	if code != 0 || !strings.Contains(out, filepath.Join(named, "auth.json")) {
		t.Fatalf("= %d %q: a folder named in the environment must win over the unit's", code, out)
	}
	if !strings.Contains(errOut, svcHome) {
		t.Errorf("a differing service folder is mentioned: %q", errOut)
	}
}

func TestRunAuthCLI_SaysWhenARunningServerReadsTheFolder(t *testing.T) {
	dir := t.TempDir()
	if err := writeMarker(dir); err != nil {
		t.Fatal(err)
	}
	installFakes(t, &fakes{euid: 1000, owner: map[string][2]int{dir: {1000, 1000}}})
	code, out, errOut := runCLI("setup-code", "--data", dir)
	if code != 0 || !strings.Contains(out, "A running server") || strings.Contains(errOut, "no running server") {
		t.Errorf("= %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestRunAuthCLI_WhenTheSwitchFails_NothingIsWritten(t *testing.T) {
	dir := t.TempDir()
	f := &fakes{euid: 0, owner: map[string][2]int{dir: {998, 997}}, becomeErr: errors.New("operation not permitted")}
	installFakes(t, f)
	code, _, errOut := runCLI("setup-code", "--data", dir)
	if code == 0 || !strings.Contains(errOut, "operation not permitted") || !strings.Contains(errOut, "owns") {
		t.Errorf("= %d %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.json")); err == nil {
		t.Error("a file was written although the switch failed: it would belong to the wrong account")
	}
}

func TestRunAuthCLI_NamingTheFolderIsBelieved_AndTheFileIsAlwaysNamed(t *testing.T) {
	root := t.TempDir()
	defer func(l func() (installedService, bool)) { locateService = l }(locateService)
	locateService = func() (installedService, bool) {
		return installedService{User: "llm-proxy", Home: "/var/lib/llm-proxy", Binary: "/usr/local/bin/llm-proxy"}, true
	}
	var out, errOut bytes.Buffer
	if code := RunAuthCLI([]string{"setup-code", "--data", root}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("= %d %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), filepath.Join(root, "auth.json")) {
		t.Errorf("the output must name the file it wrote: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "/var/lib/llm-proxy") {
		t.Errorf("a different folder from the service's must be called out: %q", errOut.String())
	}
}

// writeMarker leaves the marker a running server would: this test process stands in for it.
func writeMarker(dir string) error { return app.WriteServerMarker(dir, ":4001") }

func TestRunAuthCLI_AFolderOfAnotherAccountGetsASudoHint(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil { // not writable: stands in for the service's folder
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	installFakes(t, &fakes{euid: 1000, unit: &installedService{User: "llm-proxy", Home: dir, Binary: "/usr/local/bin/llm-proxy"}})
	code, _, errOut := runCLI("setup-code", "--data", dir)
	if code == 0 || !strings.Contains(errOut, "sudo /usr/local/bin/llm-proxy auth setup-code") {
		t.Errorf("= %d %q: want the exact sudo command", code, errOut)
	}
}
