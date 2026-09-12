package sandbox

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestRunnerChildExec verifies the self-exec runner end-to-end on any OS: the
// parent spawns this test binary as a runner child (marker env), the child
// applies confinement (stubbed — Landlock is Linux-only) and execs the real
// argv ("sh -c echo runner-ok"), replacing the process.
func TestRunnerChildExec(t *testing.T) {
	outFile := t.TempDir() + "/rules.txt"
	cmd := exec.Command(os.Args[0], "-test.run=TestRunnerHelperProcess")
	cmd.Env = append(os.Environ(),
		runnerEnvMarker+"=1",
		rulesEnvVar+"=3 "+t.TempDir()+"/workspace",
		"LLMPROXY_SANDBOX_RULES_FILE="+outFile,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runner child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "runner-ok") {
		t.Errorf("real argv was not executed; output = %q", out)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("confinement stub never ran: %v", err)
	}
	if !strings.Contains(string(data), "workspace") {
		t.Errorf("confinement stub got wrong rules: %q", data)
	}
}

// TestRunnerHelperProcess is the runner child entry: applies the (stubbed)
// confinement, records the rules, and execs the real argv.
func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv(runnerEnvMarker) != "1" {
		return // not a runner invocation — normal test run
	}
	applyRules = func(encoded string, denyNet bool) error {
		if denyNet {
			return os.WriteFile(os.Getenv("LLMPROXY_SANDBOX_RULES_FILE"), []byte("NET_DENY\n"+encoded), 0o600)
		}
		return os.WriteFile(os.Getenv("LLMPROXY_SANDBOX_RULES_FILE"), []byte(encoded), 0o600)
	}
	real := []string{"sh", "-c", "echo runner-ok"}
	os.Args = append([]string{os.Args[0], runnerFlag}, real...)
	RunChildIfRequested()
	// Unreachable on success (exec replaces the process); if we get here the
	// runner failed closed and already exited non-zero.
	t.Fatal("runner returned without exec'ing")
}

// A stale/inherited marker variable alone must never hijack a normal backend
// startup: the runner requires BOTH the env marker and the CLI flag.
func TestRunnerMarkerWithoutFlagDoesNotHijack(t *testing.T) {
	t.Setenv(runnerEnvMarker, "1")
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{os.Args[0], "-test.run=TestRunnerMarkerWithoutFlagDoesNotHijack"}

	if RunChildIfRequested() {
		t.Fatal("runner hijacked a normal startup without the runner flag")
	}
}

// The confined process must not inherit the runner's private spawn-control
// variables (they could hijack a later startup of this binary and leak the rule
// set into the agent environment).
func TestEnvWithoutRunnerMarkers(t *testing.T) {
	t.Setenv(runnerEnvMarker, "1")
	t.Setenv(rulesEnvVar, "3 /workspace")
	t.Setenv(netDenyEnvVar, "1")
	t.Setenv("KEEP_ME", "yes")

	env := envWithoutRunnerMarkers()
	sawKeep := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "LLMPROXY_SANDBOX_") {
			t.Errorf("runner-private variable leaked into the child env: %q", kv)
		}
		if kv == "KEEP_ME=yes" {
			sawKeep = true
		}
	}
	if !sawKeep {
		t.Error("unrelated environment variables must be preserved")
	}
}

// The FSIZE backstop is parsed from the parent-set env var; absent = no cap,
// malformed must fail closed (the runner exits rather than running uncapped).
func TestChildLimitsFromEnv(t *testing.T) {
	t.Setenv(fsizeEnvVar, "")
	if got, err := childLimitsFromEnv(); err != nil || got.MaxFileSizeBytes != 0 {
		t.Fatalf("unset fsize = (%+v, %v), want zero limit and no error", got, err)
	}
	t.Setenv(fsizeEnvVar, "2147483648")
	if got, err := childLimitsFromEnv(); err != nil || got.MaxFileSizeBytes != 2147483648 {
		t.Fatalf("parsed fsize = (%+v, %v), want 2147483648", got, err)
	}
	t.Setenv(fsizeEnvVar, "not-a-number")
	if _, err := childLimitsFromEnv(); err == nil {
		t.Fatal("malformed fsize must be an error")
	}
	t.Setenv(fsizeEnvVar, "-5")
	if _, err := childLimitsFromEnv(); err == nil {
		t.Fatal("negative fsize must be an error")
	}
}

// appendChildEnv must honor os/exec's nil-Env-means-inherit contract: a nil
// Env becomes the parent environment plus the runner vars, never the runner
// vars alone (which would run the child with no PATH/HOME).
func TestAppendChildEnvPreservesInheritedEnvironment(t *testing.T) {
	t.Setenv("SANDBOX_INHERIT_PROBE", "present")

	var inherited exec.Cmd // Env nil = inherit
	appendChildEnv(&inherited, []string{"RUNNER=1"})
	if !slices.Contains(inherited.Env, "SANDBOX_INHERIT_PROBE=present") {
		t.Errorf("inherited environment dropped: %v", inherited.Env)
	}
	if !slices.Contains(inherited.Env, "RUNNER=1") {
		t.Errorf("runner env not appended: %v", inherited.Env)
	}

	explicit := exec.Cmd{Env: []string{"ONLY=1"}} // explicit env is authoritative
	appendChildEnv(&explicit, []string{"RUNNER=1"})
	if slices.Contains(explicit.Env, "SANDBOX_INHERIT_PROBE=present") {
		t.Errorf("explicit Env must not be replaced by the parent env: %v", explicit.Env)
	}
	if !slices.Contains(explicit.Env, "ONLY=1") || !slices.Contains(explicit.Env, "RUNNER=1") {
		t.Errorf("explicit env not augmented: %v", explicit.Env)
	}
}

// Baseline for the spawn-time serialization cost (plan §9 T7): EncodeProfile
// runs once per jailed spawn on the parent side; DecodeProfile runs once in the
// child before exec. Record numbers on the target host, not blind targets.
func BenchmarkEncodeProfile(b *testing.B) {
	p := Profile{Rules: make([]Rule, 0, 64)}
	for i := 0; i < 64; i++ {
		p.Rules = append(p.Rules, Rule{Path: "/workspace/dir" + string(rune('a'+i%26)) + "/sub/deeper", Perm: PermRead | PermWrite})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EncodeProfile(p)
	}
}
