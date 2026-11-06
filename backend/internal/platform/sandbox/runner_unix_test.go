package sandbox

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A spawn that asked for the Unix-socket denial must fail closed when this build cannot apply it: the real command never runs.
func TestRunner_FailsClosedWhenTheUnixDenialCannotBeApplied(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestRunnerUnixDenyMissingHelper")
	cmd.Env = append(os.Environ(), runnerEnvMarker+"=1", unixDenyEnvVar+"=1", "LLMPROXY_UNIX_MISSING=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("the runner exec'd the command although the denial was not applied:\n%s", out)
	}
	if strings.Contains(string(out), "SHOULD-NOT-RUN") {
		t.Fatalf("the real command ran:\n%s", out)
	}
	if !strings.Contains(string(out), "unix socket") {
		t.Fatalf("the failure should say what could not be applied:\n%s", out)
	}
}

func TestRunnerUnixDenyMissingHelper(t *testing.T) {
	if os.Getenv("LLMPROXY_UNIX_MISSING") != "1" {
		return
	}
	applyRules = func(string, bool) error { return nil }
	applyUnixDeny = nil
	os.Args = []string{os.Args[0], runnerFlag, "sh", "-c", "echo SHOULD-NOT-RUN"}
	RunChildIfRequested()
	t.Fatal("runner returned")
}

func TestEnvWithoutRunnerMarkers_AlsoDropsTheUnixDenyMarker(t *testing.T) {
	t.Setenv(unixDenyEnvVar, "1")
	for _, kv := range envWithoutRunnerMarkers() {
		if strings.HasPrefix(kv, unixDenyEnvVar+"=") {
			t.Fatalf("the marker leaked into the confined process: %q", kv)
		}
	}
}
