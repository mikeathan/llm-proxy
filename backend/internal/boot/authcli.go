package boot

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"llm-proxy/internal/app"
	"llm-proxy/internal/platform/auth"
	"llm-proxy/internal/platform/env"
	"llm-proxy/internal/platform/paths"
)

const authUsage = `usage: llm-proxy auth <command> [--data DIR]

commands:
  setup-code   print a one-time code (valid 15 minutes) to set the passphrase in the UI
  status       say whether sign-in is set up
  reset        remove the passphrase, every session and every script token (asks first; --yes skips the question)

These run on the server, as the server's OS user, and change the sign-in file directly: that is what proves the person
running them may. There is deliberately no web route to any of them.`

const resetConfirmation = "reset"

// RunAuthCLI runs `llm-proxy auth …` and returns the process exit code. It works on the sign-in file itself, so it needs no
// running server, and a running server notices the change.
//
// It acts on the data folder the server would use with the same flags and environment (.env files are loaded first, as
// the server does, so `go run . auth …` and the installed service resolve alike). When run as root it becomes the account
// that owns that folder before writing, so the file is readable by the server; with a service installed and no folder
// named, root uses the service's folder. Afterwards it says whether a live server reads the folder, so a setup code
// written somewhere the server does not look is reported at once.
func RunAuthCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, authUsage)
		return 2
	}
	command, rest := args[0], args[1:]
	flags := flag.NewFlagSet("auth "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	data := flags.String("data", "", "the data directory (as for the server)")
	yes := flags.Bool("yes", false, "do not ask before a reset")
	if err := flags.Parse(rest); err != nil {
		return 2
	}
	if command != "setup-code" && command != "status" && command != "reset" {
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", command, authUsage)
		return 2
	}

	env.LoadEnv()
	unit, haveUnit := locateService()
	dir, err := chooseFolder(*data, unit, haveUnit)
	if err == nil {
		err = actAsOwner(dir, unit, haveUnit)
	}
	var svc *auth.Service
	var file string
	if err == nil {
		svc, file, err = openAuth(dir)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if currentUID() != 0 && errors.Is(err, fs.ErrPermission) {
			fmt.Fprintf(stderr, "That folder belongs to another account. Run it with sudo, which acts as the account that owns it:\n  sudo %s auth %s\n", binaryName(unit, haveUnit), strings.TrimSpace(command+" "+strings.Join(rest, " ")))
		}
		return 1
	}

	code := runCommand(command, svc, file, *yes, stdin, stdout, stderr)
	reportServer(dir, command, rest, unit, haveUnit, stdout, stderr)
	return code
}

// binaryName is the program to name in a hint: the installed service's, else the name it is usually found under.
func binaryName(unit installedService, haveUnit bool) string {
	if haveUnit {
		return unit.Binary
	}
	return "llm-proxy"
}

func runCommand(command string, svc *auth.Service, file string, yes bool, stdin io.Reader, stdout, stderr io.Writer) int {
	switch command {
	case "setup-code":
		return setupCode(svc, file, stdout, stderr)
	case "status":
		return authStatus(svc, file, stdout, stderr)
	}
	return resetAuth(svc, file, yes, stdin, stdout, stderr)
}

// chooseFolder is the data folder the command acts on: the one named (--data, or LLM_PROXY_HOME from the environment or a
// .env file), else, for root with a service installed, the service's, else the default the server would use.
func chooseFolder(data string, unit installedService, haveUnit bool) (string, error) {
	named := data != "" || os.Getenv(paths.EnvHome) != ""
	if !named && haveUnit && currentUID() == 0 {
		data = unit.Home
	}
	return paths.Locate(data)
}

// actAsOwner makes a root process the account that owns the folder (the server creates its folder 0700, so the owner is
// the account that reads it), or, for a folder that does not exist yet and is the service's, the service's account. Anyone
// else acts as themselves.
func actAsOwner(dir string, unit installedService, haveUnit bool) error {
	if currentUID() != 0 {
		return nil
	}
	uid, gid, ok := ownerOf(dir)
	if !ok && haveUnit && dir == unit.Home {
		uid, gid, ok = idsOf(unit.User)
	}
	if !ok || uid == 0 {
		return nil
	}
	if err := becomeUser(uid, gid); err != nil {
		return fmt.Errorf("could not switch to the account that owns %s (user id %d): %w", dir, uid, err)
	}
	return nil
}

// reportServer says whether a live server reads the folder, using the marker a running server leaves in it. A folder no
// server reads is the usual way a setup code goes missing, so it is called out with the command that acts on the service.
func reportServer(dir, command string, rest []string, unit installedService, haveUnit bool, stdout, stderr io.Writer) {
	if m, ok := app.ReadServerMarker(dir); ok && m.Alive() {
		fmt.Fprintf(stdout, "A running server (process %d) reads this folder.\n", m.PID)
		return
	}
	fmt.Fprintf(stderr, "\nWarning: no running server was found using %s. (A server older than 0.18.2 leaves no marker, so it cannot be checked.)\n", dir)
	if haveUnit && unit.Home != dir {
		fmt.Fprintf(stderr, "The service installed on this machine uses %s. To act on that one:\n  sudo %s auth %s\n", unit.Home, unit.Binary, strings.TrimSpace(command+" "+strings.Join(rest, " ")))
	}
}

func openAuth(dir string) (*auth.Service, string, error) {
	p, err := paths.Resolve(dir)
	if err != nil {
		return nil, "", fmt.Errorf("could not use the data directory: %w", err)
	}
	file := app.AuthFilePath(p.ConfigDir)
	return auth.NewService(auth.OpenStore(file), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig()), file, nil
}

func setupCode(svc *auth.Service, file string, stdout, stderr io.Writer) int {
	code, err := svc.IssueSetupCode()
	if errors.Is(err, auth.ErrAlreadySetUp) {
		fmt.Fprintln(stderr, "Sign-in is already set up. To start again, run: llm-proxy auth reset")
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Setup code:  %s\n\nOpen the interface and enter this code with the passphrase you choose.\nIt works once and expires in 15 minutes.\n\nSign-in file: %s\n", code, file)
	return 0
}

func authStatus(svc *auth.Service, file string, stdout, stderr io.Writer) int {
	st, err := svc.State()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if st.SetUp {
		fmt.Fprintf(stdout, "Sign-in is set up. %d script token(s).\nSign-in file: %s\n", len(svc.Tokens()), file)
		return 0
	}
	fmt.Fprintf(stdout, "Sign-in is not set up: the admin interface refuses every request until it is. Run: llm-proxy auth setup-code\nSign-in file: %s\n", file)
	return 0
}

func resetAuth(svc *auth.Service, file string, yes bool, stdin io.Reader, stdout, stderr io.Writer) int {
	if !yes {
		fmt.Fprintf(stdout, "This removes the passphrase, signs everyone out and deletes every script token. The admin interface is then locked until you set a new passphrase.\nType %q to continue: ", resetConfirmation)
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		if strings.TrimSpace(line) != resetConfirmation {
			fmt.Fprintln(stderr, "\nNothing was changed.")
			return 1
		}
	}
	if err := svc.Reset(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Sign-in was reset and the admin interface is locked. To set a new passphrase run: llm-proxy auth setup-code\nSign-in file: %s\n", file)
	return 0
}
