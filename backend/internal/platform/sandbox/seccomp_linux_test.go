//go:build linux && (amd64 || arm64)

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// What the child prints, one word per fact, so the parent can read it as a protocol.
func probeUnixFacts() string {
	var out []string
	fact := func(name string, err error) {
		if err == nil {
			out = append(out, name+"=ok")
		} else {
			out = append(out, name+"="+err.Error())
		}
	}
	if fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0); err == nil {
		_ = syscall.Close(fd)
		fact("unix_socket", nil)
	} else {
		fact("unix_socket", err)
	}
	if fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0); err == nil {
		_ = syscall.Close(fds[0])
		_ = syscall.Close(fds[1])
		fact("unix_socketpair", nil)
	} else {
		fact("unix_socketpair", err)
	}
	if fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0); err == nil {
		_ = syscall.Close(fd)
		fact("inet_socket", nil)
	} else {
		fact("inet_socket", err)
	}
	// io_uring can open sockets without socket(2); the filter refuses to set one up (numbers 0 and a null pointer: the call
	// is refused before it looks at them, or fails with EFAULT/EINVAL if it were allowed).
	_, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, 0, 0, 0)
	out = append(out, "io_uring_setup="+errno.Error())
	return strings.Join(out, " ")
}

// TestUnixDenyHelper is the child: with the filter on (marker set by the parent) it reports what it can still do.
func TestUnixDenyHelper(t *testing.T) {
	if os.Getenv("LLMPROXY_UNIX_PROBE") != "1" {
		return
	}
	if os.Getenv("LLMPROXY_UNIX_PROBE_APPLY") == "1" {
		if err := applyUnixSocketDeny(); err != nil {
			t.Fatalf("applyUnixSocketDeny: %v", err)
		}
	}
	fmt.Println("FACTS", probeUnixFacts())
}

func runProbe(t *testing.T, apply bool) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestUnixDenyHelper")
	cmd.Env = append(os.Environ(), "LLMPROXY_UNIX_PROBE=1")
	if apply {
		cmd.Env = append(cmd.Env, "LLMPROXY_UNIX_PROBE_APPLY=1")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe child failed: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "FACTS "); ok {
			return rest
		}
	}
	t.Fatalf("probe printed no facts:\n%s", out)
	return ""
}

func TestUnixSocketDeny_RefusesUnixSocketsAndIoUringAndNothingElse(t *testing.T) {
	without := runProbe(t, false)
	if !strings.Contains(without, "unix_socket=ok") {
		t.Skipf("this host does not let a plain child make a Unix socket (%s), so the filter cannot be shown", without)
	}
	got := runProbe(t, true)
	for _, want := range []string{
		"unix_socket=permission denied", // the point: a path-based socket (the system D-Bus, a control socket) cannot be reached
		"unix_socketpair=ok",            // an anonymous pair cannot connect to a path, and ordinary programs (node, python) make them
		"inet_socket=ok",                // network is the Landlock network rule's business, not this filter's
		"io_uring_setup=operation not permitted",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("facts = %q, want %q", got, want)
		}
	}
}

// The filter must survive the exec of the real command, or it would confine only the runner.
func TestUnixSocketDeny_SurvivesTheRunnersExec(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestUnixDenyRunnerHelper")
	cmd.Env = append(os.Environ(), runnerEnvMarker+"=1", unixDenyEnvVar+"=1", rulesEnvVar+"=", "LLMPROXY_UNIX_RUNNER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runner child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "unix_socket=permission denied") {
		t.Fatalf("the exec'd command could still make a Unix socket:\n%s", out)
	}
}

// TestUnixDenyRunnerHelper is the runner child: Landlock is stubbed, the Unix filter is the real one, and the real argv is this
// test binary again, which reports what it can do.
func TestUnixDenyRunnerHelper(t *testing.T) {
	if os.Getenv("LLMPROXY_UNIX_RUNNER") != "1" {
		return
	}
	if os.Getenv("LLMPROXY_UNIX_AFTER_EXEC") == "1" {
		fmt.Println("FACTS", probeUnixFacts())
		return
	}
	applyRules = func(string, bool) error { return nil }
	_ = os.Setenv("LLMPROXY_UNIX_AFTER_EXEC", "1")
	os.Args = []string{os.Args[0], runnerFlag, os.Args[0], "-test.run=TestUnixDenyRunnerHelper"}
	RunChildIfRequested()
	t.Fatal("runner returned without exec'ing")
}

func TestUnixSocketFilter_StartsByCheckingTheArchitectureAndEndsByAllowing(t *testing.T) {
	prog := unixSocketFilter()
	if len(prog) < 6 {
		t.Fatalf("program has %d instructions", len(prog))
	}
	if prog[0].Code != bpfLdAbsW || prog[0].K != seccompArchOffset {
		t.Errorf("first instruction %+v must load the architecture", prog[0])
	}
	if last := prog[len(prog)-1]; last.Code != bpfRet || last.K != seccompRetAllow {
		t.Errorf("last instruction %+v must allow: anything not named is allowed", last)
	}
	for i, ins := range prog {
		if ins.Code&0x07 == bpfJmpClass && i+1+int(ins.Jt) >= len(prog) || ins.Code&0x07 == bpfJmpClass && i+1+int(ins.Jf) >= len(prog) {
			t.Errorf("instruction %d jumps past the end: %+v", i, ins)
		}
	}
}

// denySeccompSyscall is what the service's own systemd unit does to a child: its SystemCallFilter allows @system-service and three
// Landlock calls, and the seccomp(2) syscall is in neither (systemd keeps it in @sandbox), so the unit refuses it. A filter that can
// only be installed through seccomp(2) works in a container and fails on the real service (vertex, 2026-11-06: nmap died with
// "seccomp: install the unix socket filter: operation not permitted" and the nightly audit with it).
func denySeccompSyscall(t *testing.T) {
	t.Helper()
	prog := assemble([]ins{
		{code: bpfLdAbsW, k: seccompNrOffset},
		{code: bpfJeqK, k: unix.SYS_SECCOMP, jf: "allow"},
		{code: bpfRet, k: seccompRetErrno | uint32(unix.EPERM)},
		{label: "allow", code: bpfRet, k: seccompRetAllow},
	})
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&fprog)), 0, 0); err != nil {
		t.Skipf("this host will not let the test stand in for the unit's filter: %v", err)
	}
	// Proof that the stand-in works: the syscall is now refused.
	if _, _, errno := unix.Syscall(unix.SYS_SECCOMP, 99, 0, 0); errno != unix.EPERM {
		t.Fatalf("the stand-in filter did not refuse seccomp(2): %v", errno)
	}
}

func TestUnixDenyUnderTheUnitsFilterHelper(t *testing.T) {
	if os.Getenv("LLMPROXY_UNIX_UNDER_UNIT") != "1" {
		return
	}
	denySeccompSyscall(t)
	if err := applyUnixSocketDeny(); err != nil {
		t.Fatalf("the Unix-socket filter cannot be installed under a filter that refuses seccomp(2): %v", err)
	}
	fmt.Println("FACTS", probeUnixFacts())
}

func TestUnixSocketDeny_InstallsUnderTheServiceUnitsOwnSyscallFilter(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestUnixDenyUnderTheUnitsFilterHelper")
	cmd.Env = append(os.Environ(), "LLMPROXY_UNIX_UNDER_UNIT=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "unix_socket=permission denied") {
		t.Fatalf("the filter was not in force:\n%s", out)
	}
}
