package boot

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
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
// running server, and a running server notices the change. With the service installed under systemd it acts on the
// service's folder as the service's account, whoever types it and from wherever, so a setup code can never land in a file
// the server does not read.
func RunAuthCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, authUsage)
		return 2
	}
	command, rest := args[0], args[1:]
	fs := flag.NewFlagSet("auth "+command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	data := fs.String("data", "", "the data directory (as for the server)")
	yes := fs.Bool("yes", false, "do not ask before a reset")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	switch command {
	case "setup-code", "status", "reset":
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", command, authUsage)
		return 2
	}

	dir, code, done := chooseFolder(command, rest, *data, stderr)
	if done {
		return code
	}
	svc, file, err := openAuth(dir)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	switch command {
	case "setup-code":
		return setupCode(svc, file, stdout, stderr)
	case "status":
		return authStatus(svc, file, stdout, stderr)
	}
	return resetAuth(svc, file, *yes, stdin, stdout, stderr)
}

// chooseFolder decides which data folder the command acts on. done is true when the command has been dealt with (run as the
// service's account, or refused with the exact command to use) and code is its exit code.
func chooseFolder(command string, rest []string, data string, stderr io.Writer) (dir string, code int, done bool) {
	explicit := data != "" || os.Getenv(paths.EnvHome) != ""
	svc, found := locateService()
	where, err := paths.Locate(data)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return "", 1, true
	}
	svcUID, _ := uidOf(svc.User)
	switch planAuth(explicit, svc, found, currentUID(), svcUID, where) {
	case actAsService:
		return actAs(command, rest, svc, svcUID, stderr)
	case actRefuse:
		fmt.Fprintf(stderr, "The service on this machine runs as %q with its data in %s, not where this command would look (%s).\nRun it with sudo, which uses the service's account and folder:\n  sudo %s auth %s\n",
			svc.User, svc.Home, where, svc.Binary, strings.TrimSpace(command+" "+strings.Join(rest, " ")))
		return "", 1, true
	}
	if found && explicit && where != svc.Home {
		fmt.Fprintf(stderr, "note: the service installed on this machine uses %s, not %s.\n", svc.Home, where)
	}
	return data, 0, false
}

// actAs makes this process the service's account (when it is not already), so the file it writes is the service's own, and
// the command then runs here on the service's folder. Only root can do that; anyone else is told the exact command.
func actAs(command string, rest []string, svc installedService, svcUID int, stderr io.Writer) (string, int, bool) {
	if currentUID() != svcUID {
		if err := becomeService(svc); err != nil {
			fmt.Fprintf(stderr, "Could not switch to the service's account (%v). Run it by hand:\n  %s\n", err, handByHand(svc, command, rest))
			return "", 1, true
		}
	}
	return svc.Home, 0, false
}

func openAuth(dataDir string) (*auth.Service, string, error) {
	env.LoadEnv()
	p, err := paths.Resolve(dataDir)
	if err != nil {
		return nil, "", fmt.Errorf("could not find the data directory: %w", err)
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
	if st.Required {
		fmt.Fprintf(stdout, "Sign-in is set up. %d script token(s).\nSign-in file: %s\n", len(svc.Tokens()), file)
		return 0
	}
	fmt.Fprintf(stdout, "Sign-in is not set up: the admin interface is open to anyone who can reach it. Run: llm-proxy auth setup-code\nSign-in file: %s\n", file)
	return 0
}

func resetAuth(svc *auth.Service, file string, yes bool, stdin io.Reader, stdout, stderr io.Writer) int {
	if !yes {
		fmt.Fprintf(stdout, "This removes the passphrase, signs everyone out and deletes every script token.\nType %q to continue: ", resetConfirmation)
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
	fmt.Fprintf(stdout, "Sign-in was reset. Run: llm-proxy auth setup-code\nSign-in file: %s\n", file)
	return 0
}
