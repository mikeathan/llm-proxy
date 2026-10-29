package boot

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
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

	svc, err := openAuth(*data)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	switch command {
	case "setup-code":
		return setupCode(svc, stdout, stderr)
	case "status":
		return authStatus(svc, stdout, stderr)
	case "reset":
		return resetAuth(svc, *yes, stdin, stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q\n%s\n", command, authUsage)
	return 2
}

func openAuth(dataDir string) (*auth.Service, error) {
	env.LoadEnv()
	p, err := paths.Resolve(dataDir)
	if err != nil {
		return nil, fmt.Errorf("could not find the data directory: %w", err)
	}
	return auth.NewService(auth.OpenStore(app.AuthFilePath(p.ConfigDir)), auth.NewHasher(auth.DefaultParams()), auth.DefaultConfig()), nil
}

func setupCode(svc *auth.Service, stdout, stderr io.Writer) int {
	code, err := svc.IssueSetupCode()
	if errors.Is(err, auth.ErrAlreadySetUp) {
		fmt.Fprintln(stderr, "Sign-in is already set up. To start again, run: llm-proxy auth reset")
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Setup code:  %s\n\nOpen the interface and enter this code with the passphrase you choose.\nIt works once and expires in 15 minutes.\n", code)
	return 0
}

func authStatus(svc *auth.Service, stdout, stderr io.Writer) int {
	st, err := svc.State()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if st.Required {
		fmt.Fprintf(stdout, "Sign-in is set up. %d script token(s).\n", len(svc.Tokens()))
		return 0
	}
	fmt.Fprintln(stdout, "Sign-in is not set up: the admin interface is open to anyone who can reach it. Run: llm-proxy auth setup-code")
	return 0
}

func resetAuth(svc *auth.Service, yes bool, stdin io.Reader, stdout, stderr io.Writer) int {
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
	fmt.Fprintln(stdout, "Sign-in was reset. Run: llm-proxy auth setup-code")
	return 0
}
