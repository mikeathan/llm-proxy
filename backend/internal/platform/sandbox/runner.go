package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"llm-proxy/internal/platform/process"
)

// Self-exec sandbox-runner (plan §4.3): Go's os/exec has no pre-exec hook and
// Landlock is self-confining (landlock_restrict_self applies only to the
// calling process), so a sandboxed child is launched as THIS binary with a
// marker, applies the confinement in-process, then syscall.Exec's the real
// argv. The child never runs a byte of the requested command unconfined.

const (
	runnerEnvMarker = "LLMPROXY_SANDBOX_RUNNER"
	rulesEnvVar     = "LLMPROXY_SANDBOX_RULES"
	runnerFlag      = "-llm-proxy-sandbox-child"
	// netDenyEnvVar marks a runner invocation that must deny new TCP
	// binds/connects (network-off shell, plan D8). Providers set it only when
	// the kernel can express the denial, so a spawn is never failed by a
	// confinement option the host cannot apply — Effective reports the gap.
	netDenyEnvVar = "LLMPROXY_SANDBOX_NET_DENY"
	// unixDenyEnvVar marks a runner invocation that must refuse socket(AF_UNIX) (a seccomp filter), because the Landlock jail does
	// not control the connection to a path-based Unix socket. Providers set it only when the filter can be applied here; a runner
	// that is asked for it and cannot apply it fails closed.
	unixDenyEnvVar = "LLMPROXY_SANDBOX_UNIX_DENY"
	// fsizeEnvVar carries the per-file RLIMIT_FSIZE backstop (bytes) derived
	// from the max_storage_gb accounting boundary; unset/0 = no cap.
	fsizeEnvVar = "LLMPROXY_SANDBOX_FSIZE_BYTES"
)

// EncodeProfile serializes the grant profile into the rules env var: one
// "perm path" per line, perm = read(1)|write(2)|traverse(4).
func EncodeProfile(p Profile) string {
	var b strings.Builder
	for _, r := range p.Rules {
		var perm int
		if r.Perm&PermRead != 0 {
			perm |= 1
		}
		if r.Perm&PermWrite != 0 {
			perm |= 2
		}
		if r.Perm&PermTraverse != 0 {
			perm |= 4
		}
		b.WriteString(strconv.Itoa(perm))
		b.WriteByte(' ')
		b.WriteString(r.Path)
		b.WriteByte('\n')
	}
	return b.String()
}

// applyRules is set by OS init()s to the mechanism's confinement application
// (Linux: Landlock; other OSes: nil ⇒ runner fails closed). denyNet requests OS
// network denial for the child (network-off spawns, plan D8); mechanisms that
// cannot express it must fail, never silently skip. Overridable in tests.
var applyRules func(encoded string, denyNet bool) error

// applyUnixDeny installs the filter that refuses socket(AF_UNIX) (seccomp_linux.go sets it on the architectures that have it). Nil
// means this build cannot, and a runner asked for it fails closed. Overridable in tests.
var applyUnixDeny func() error

// childLimitsFromEnv derives the child rlimits the parent requested. Only the
// measured-safe per-file FSIZE backstop is wired by default; memory (RLIMIT_AS/
// DATA) and NPROC are deliberately absent — the systemd unit's cgroup is the
// real memory lever for the service, and a per-UID NPROC cap on a shared uid
// self-DoSes (plan D5/R7). macOS cannot set AS/DATA at all (measured).
func childLimitsFromEnv() (process.ChildLimits, error) {
	raw := os.Getenv(fsizeEnvVar)
	if raw == "" {
		return process.ChildLimits{}, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return process.ChildLimits{}, fmt.Errorf("invalid %s=%q", fsizeEnvVar, raw)
	}
	return process.ChildLimits{MaxFileSizeBytes: n}, nil
}

// runnerEnvKeys are the runner's private spawn-control variables. They must not
// leak into the confined process (or its children): the markers can hijack a
// later startup of this binary, and the rule set is internal state.
var runnerEnvKeys = map[string]bool{
	runnerEnvMarker: true,
	rulesEnvVar:     true,
	netDenyEnvVar:   true,
	unixDenyEnvVar:  true,
	fsizeEnvVar:     true,
}

// envWithoutRunnerMarkers returns the process environment minus the runner's
// private variables (used for the syscall.Exec of the real argv).
func envWithoutRunnerMarkers() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if ok && runnerEnvKeys[key] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// appendChildEnv adds env entries to cmd while preserving os/exec's "nil Env
// means inherit the parent environment" contract. Appending to a nil slice
// would otherwise hand the child ONLY these entries (no PATH, no HOME).
func appendChildEnv(cmd *exec.Cmd, env []string) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, env...)
}

// RunChildIfRequested detects the runner marker at process start and, when
// present, applies confinement and execs the real command. It returns false
// when this is not a runner invocation (normal process start). It never
// returns on the child path: success replaces the process via syscall.Exec;
// any failure prints and exits non-zero — an unconfined child is never run.
func RunChildIfRequested() bool {
	if os.Getenv(runnerEnvMarker) != "1" {
		return false
	}
	// Require BOTH the env marker and the CLI flag: a stale/inherited variable
	// alone must never hijack a normal backend startup.
	if len(os.Args) < 2 || os.Args[1] != runnerFlag {
		return false
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "sandbox-runner: missing real argv")
		os.Exit(1)
	}
	if applyRules == nil {
		fmt.Fprintln(os.Stderr, "sandbox-runner: no confinement mechanism on this build")
		os.Exit(1)
	}
	if err := confineSelf(); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-runner: %v\n", err)
		os.Exit(1)
	}
	realArgv := os.Args[2:]
	bin, err := exec.LookPath(realArgv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-runner: cannot resolve %s: %v\n", realArgv[0], err)
		os.Exit(1)
	}
	if err := syscall.Exec(bin, realArgv, envWithoutRunnerMarkers()); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-runner: exec failed: %v\n", err)
		os.Exit(1)
	}
	return false // unreachable
}

// confineSelf applies, in this order, the resource limits the parent asked for, the filesystem jail (and the TCP denial when asked)
// and the Unix-socket denial when asked. Any step that was requested and cannot be applied is an error: the runner then exits
// before the real command runs, so an unconfined child is never started.
func confineSelf() error {
	limits, err := childLimitsFromEnv()
	if err != nil {
		return fmt.Errorf("bad child limits: %w", err)
	}
	if _, skipped, err := process.ApplyChildLimits(limits); err != nil {
		// A requested resource cap that cannot be applied must fail closed: silently running without it would misrepresent enforcement.
		return fmt.Errorf("applying child limits: %w", err)
	} else if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr, "sandbox-runner: limits skipped by the OS: %v\n", skipped)
	}
	if err := applyRules(os.Getenv(rulesEnvVar), os.Getenv(netDenyEnvVar) == "1"); err != nil {
		return fmt.Errorf("confinement failed: %w", err)
	}
	if os.Getenv(unixDenyEnvVar) != "1" {
		return nil
	}
	if applyUnixDeny == nil {
		return errors.New("the unix socket denial was requested but this build cannot apply it")
	}
	if err := applyUnixDeny(); err != nil {
		return fmt.Errorf("unix socket denial failed: %w", err)
	}
	return nil
}
