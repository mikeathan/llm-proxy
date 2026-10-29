package boot

import (
	"bytes"
	"errors"
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

func TestPlanAuth(t *testing.T) {
	svc := installedService{User: "llm-proxy", Home: "/var/lib/llm-proxy", Binary: "/usr/local/bin/llm-proxy"}
	const svcUID, rootUID, otherUID = 998, 0, 1000
	cases := []struct {
		name     string
		explicit bool
		found    bool
		euid     int
		where    string
		want     authAction
	}{
		{"no installed service: use what this command resolves", false, false, otherUID, "/home/me/.config/llm-proxy", actHere},
		{"the operator named a directory: believe them", true, true, otherUID, "/somewhere", actHere},
		{"already the service's account and folder", false, true, svcUID, "/var/lib/llm-proxy", actHere},
		{"root, with a service installed: act as the service's user", false, true, rootUID, "/root/.config/llm-proxy", actAsService},
		{"someone else, with a service installed: tell them how", false, true, otherUID, "/home/me/.config/llm-proxy", actRefuse},
		{"the service's own user but the wrong folder (no env set)", false, true, svcUID, "/home/llm-proxy/.config/llm-proxy", actAsService},
	}
	for _, c := range cases {
		got := planAuth(c.explicit, svc, c.found, c.euid, svcUID, c.where)
		if got != c.want {
			t.Errorf("%s: = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRunAuthCLI_WithAServiceInstalledElsewhere(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LLM_PROXY_HOME", "")
	svc := installedService{User: "llm-proxy", Home: t.TempDir(), Binary: "/usr/local/bin/llm-proxy"}
	defer func(l func() (installedService, bool), b func(installedService) error, u func() int, f func(string) (int, bool)) {
		locateService, becomeService, currentUID, uidOf = l, b, u, f
	}(locateService, becomeService, currentUID, uidOf)
	locateService = func() (installedService, bool) { return svc, true }
	uidOf = func(string) (int, bool) { return 998, true }

	// Someone who is not root and not the service: nothing is created, and the message gives the exact command.
	currentUID = func() int { return 1000 }
	var out, errOut bytes.Buffer
	code := RunAuthCLI([]string{"setup-code"}, strings.NewReader(""), &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "sudo /usr/local/bin/llm-proxy auth setup-code") || !strings.Contains(errOut.String(), svc.Home) {
		t.Errorf("= %d, stderr %q: want a refusal that names the service's folder and the sudo command", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "llm-proxy")); err == nil {
		t.Error("a refused command must not create a data folder in the wrong place")
	}

	// Root: this process becomes the service's account, and the command runs on the service's folder.
	currentUID = func() int { return 0 }
	var became installedService
	becomeService = func(s installedService) error { became = s; return nil }
	out.Reset()
	errOut.Reset()
	if code := RunAuthCLI([]string{"setup-code"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("as root = %d %q", code, errOut.String())
	}
	if became.User != "llm-proxy" {
		t.Errorf("became %+v", became)
	}
	if !strings.Contains(out.String(), filepath.Join(svc.Home, "auth.json")) {
		t.Errorf("the code must be written in the service's folder: %q", out.String())
	}

	// Root, but the switch cannot be done here: say how to do it by hand, and write nothing.
	becomeService = func(installedService) error { return errors.New("not supported here") }
	other := installedService{User: "llm-proxy", Home: "/var/lib/llm-proxy", Binary: "/usr/local/bin/llm-proxy"}
	locateService = func() (installedService, bool) { return other, true }
	errOut.Reset()
	if code := RunAuthCLI([]string{"setup-code"}, strings.NewReader(""), &out, &errOut); code == 0 || !strings.Contains(errOut.String(), "sudo -u llm-proxy /usr/local/bin/llm-proxy auth setup-code --data /var/lib/llm-proxy") {
		t.Errorf("= %d %q", code, errOut.String())
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
