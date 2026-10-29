package boot

import (
	"os"
	"os/user"
	"strconv"
	"strings"
)

// unitFile is where setup.sh installs the systemd unit (docs/services/llm-proxy.service); the vendor directories are what a
// package would use.
var unitFiles = []string{
	"/etc/systemd/system/llm-proxy.service",
	"/usr/lib/systemd/system/llm-proxy.service",
	"/lib/systemd/system/llm-proxy.service",
}

const (
	envHomeName = "LLM_PROXY_HOME"
	rootUser    = "root"
)

// installedService is the server as its systemd unit runs it: the account, the data folder and the program. The command
// line finds it so that `llm-proxy auth …` works wherever it is typed, without the operator knowing the account or the
// folder.
type installedService struct {
	User   string
	Home   string // the data folder the service uses
	Binary string // the program the unit runs
}

// Replaceable in tests: how the installed service is found, how this process becomes the service's account, who is running
// this, and the numeric id of an account.
var (
	locateService = func() (installedService, bool) { return locateUnit(os.ReadFile, homeOf) }
	becomeService = becomeServiceUser
	currentUID    = os.Geteuid
	uidOf         = lookupUID
)

type authAction int

const (
	// actHere: run in this process, on the folder this command resolves.
	actHere authAction = iota
	// actAsService: the service's own account and folder are what is meant.
	actAsService
	// actRefuse: the service is somewhere this user cannot reach; say how to get there.
	actRefuse
)

// planAuth decides where an `auth` command acts. Naming a folder (--data, or LLM_PROXY_HOME in the shell) is believed.
// Otherwise, with a service installed, the command must act on the service's folder as the service's account, or it would
// write a sign-in file the service never reads, which is how a setup code goes missing.
func planAuth(explicit bool, svc installedService, found bool, euid, svcUID int, resolved string) authAction {
	switch {
	case explicit || !found:
		return actHere
	case euid == svcUID && resolved == svc.Home:
		return actHere
	case euid == 0 || euid == svcUID:
		return actAsService
	}
	return actRefuse
}

// locateUnit finds the installed service from its systemd unit file. Not being on a systemd machine, or having no such
// unit, is "not found", and the command then acts on the folder it resolves, as before. It reads a file; it starts nothing.
func locateUnit(read func(path string) ([]byte, error), homeOf func(user string) (string, error)) (installedService, bool) {
	for _, path := range unitFiles {
		if data, err := read(path); err == nil {
			return parseUnit(string(data), homeOf)
		}
	}
	return installedService{}, false
}

func homeOf(name string) (string, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func lookupUID(name string) (int, bool) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, false
	}
	id, err := strconv.Atoi(u.Uid)
	return id, err == nil
}

// unitSettings are the three settings of a unit's [Service] section the command needs.
type unitSettings struct {
	user, execStart string
	env             []string
}

// readUnit collects User, Environment (every line, every KEY=VALUE) and the first ExecStart from a unit file's text.
func readUnit(text string) unitSettings {
	var u unitSettings
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") || strings.HasPrefix(key, ";") {
			continue
		}
		switch key {
		case "User":
			u.user = strings.TrimSpace(value)
		case "Environment":
			u.env = append(u.env, strings.Fields(value)...)
		case "ExecStart":
			if u.execStart == "" {
				u.execStart = strings.TrimSpace(value)
			}
		}
	}
	return u
}

// parseUnit reads a systemd unit's [Service] settings. The data folder is, in the order the server itself uses: --data in
// the unit's command line, LLM_PROXY_HOME in its environment, then the account's own ~/.config/llm-proxy.
func parseUnit(text string, homeOf func(string) (string, error)) (installedService, bool) {
	u := readUnit(text)
	argv := strings.Fields(strings.TrimLeft(u.execStart, "@-:+!"))
	if len(argv) == 0 {
		return installedService{}, false
	}
	svc := installedService{User: u.user, Binary: argv[0], Home: dataFlag(argv[1:])}
	if svc.User == "" {
		svc.User = rootUser
	}
	if svc.Home == "" {
		svc.Home = environmentValue(u.env, envHomeName)
	}
	if svc.Home == "" {
		home, err := homeOf(svc.User)
		if err != nil {
			return installedService{}, false
		}
		svc.Home = home + "/.config/llm-proxy"
	}
	return svc, true
}

// dataFlag returns the folder given with --data (or --data=…) in a command line.
func dataFlag(argv []string) string {
	for i, a := range argv {
		switch {
		case a == "--data" || a == "-data":
			if i+1 < len(argv) {
				return argv[i+1]
			}
		case strings.HasPrefix(a, "--data="):
			return strings.TrimPrefix(a, "--data=")
		case strings.HasPrefix(a, "-data="):
			return strings.TrimPrefix(a, "-data=")
		}
	}
	return ""
}

func environmentValue(env []string, name string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(strings.Trim(kv, `"'`), "="); ok && k == name {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// handByHand is what to type when this command cannot switch to the service's account itself.
func handByHand(svc installedService, command string, rest []string) string {
	parts := append([]string{"sudo", "-u", svc.User, svc.Binary, "auth", command}, rest...)
	return strings.Join(append(parts, "--data", svc.Home), " ")
}
