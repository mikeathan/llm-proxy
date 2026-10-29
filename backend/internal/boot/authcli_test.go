package boot

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/app"
	"llm-proxy/internal/platform/auth"
)

func runAuth(t *testing.T, root, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := RunAuthCLI(append(args, "--data", root), strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestAuthCLI_SetupCodeThenStatusThenReset(t *testing.T) {
	root := t.TempDir()

	code, out, _ := runAuth(t, root, "", "setup-code")
	if code != 0 {
		t.Fatalf("setup-code = %d", code)
	}
	var setup string
	for _, f := range strings.Fields(out) {
		if len(f) == 9 && f[4] == '-' {
			setup = f
		}
	}
	if setup == "" || !strings.Contains(out, "15 minutes") {
		t.Fatalf("the code and its lifetime must be printed: %q", out)
	}

	// The server, a different process on the same file, accepts that code.
	svc := auth.NewService(auth.OpenStore(app.AuthFilePath(root)), auth.NewHasher(auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}), auth.DefaultConfig())
	if err := svc.Setup(context.Background(), setup, "correct horse battery", "cli-test"); err != nil {
		t.Fatalf("the server did not accept the code the CLI made: %v", err)
	}

	if code, _, errOut := runAuth(t, root, "", "setup-code"); code == 0 || !strings.Contains(errOut, "reset") {
		t.Errorf("once set up, a new code is refused and the message says how to start again: %d %q", code, errOut)
	}
	if code, out, _ := runAuth(t, root, "", "status"); code != 0 || !strings.Contains(out, "set up") {
		t.Errorf("status = %d %q", code, out)
	}

	if code, _, _ := runAuth(t, root, "no\n", "reset"); code == 0 {
		t.Error("reset without the typed confirmation must stop")
	}
	if st, _ := svc.State(); !st.SetUp {
		t.Error("an unconfirmed reset changed something")
	}
	if code, _, errOut := runAuth(t, root, "reset\n", "reset"); code != 0 {
		t.Fatalf("confirmed reset = %d %q", code, errOut)
	}
	if st, _ := svc.State(); st.SetUp {
		t.Error("the running server still requires sign-in after a reset")
	}
}

func TestAuthCLI_ResetWithYesFlagAndUnknownCommands(t *testing.T) {
	root := t.TempDir()
	if code, _, _ := runAuth(t, root, "", "reset", "--yes"); code != 0 {
		t.Errorf("reset --yes on a fresh install = %d", code)
	}
	if code, _, errOut := runAuth(t, root, "", "frobnicate"); code == 0 || !strings.Contains(errOut, "setup-code") {
		t.Errorf("unknown command = %d %q; the usage must list the commands", code, errOut)
	}
	if code, _, _ := RunAuthCLI(nil, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}), 0, 0; code == 0 {
		t.Error("no command must print usage and fail")
	}
}

func TestAuthCLI_NeverPrintsAStoredSecret(t *testing.T) {
	root := t.TempDir()
	_, _, _ = runAuth(t, root, "", "setup-code")
	_, out, errOut := runAuth(t, root, "", "status")
	if strings.Contains(out+errOut, "$argon2id") {
		t.Error("status printed a hash")
	}
	_ = filepath.Join(root, "auth.json")
}
