//go:build linux && (amd64 || arm64)

package sandbox

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The Unix-socket denial. Landlock (ABI 4, the kernel of Ubuntu 24.04) controls files, not the connection to a path-based Unix
// socket: a jailed child that cannot read /etc/hostname can still connect to the system D-Bus (docs/architecture/pitfalls.md 67). The
// jail cannot say no to that, so this filter does: it refuses socket(AF_UNIX, ...), which every such connection starts with. Nothing
// else is refused except io_uring_setup, which can open sockets without the socket call. socketpair is left alone: an anonymous pair
// cannot reach a path, and node, python and the shell use them for their children.
//
// The program is a few instructions built here so it can be read: check the architecture (anything but the native one is killed, so
// a 32-bit or x32 call cannot slip past by another number), refuse the two calls, allow the rest.

const (
	bpfLdAbsW   = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJeqK     = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfJsetK    = 0x45 // BPF_JMP | BPF_JSET | BPF_K
	bpfRet      = 0x06 // BPF_RET | BPF_K
	bpfJmpClass = 0x05

	seccompNrOffset   = 0  // seccomp_data.nr
	seccompArchOffset = 4  // seccomp_data.arch
	seccompArg0Offset = 16 // seccomp_data.args[0], low word (both supported architectures are little-endian)

	seccompRetAllow = 0x7fff0000
	seccompRetKill  = 0x80000000 // SECCOMP_RET_KILL_PROCESS
	seccompRetErrno = 0x00050000

	x32SyscallBit = 0x40000000
	afUnix        = 1
)

// ins is one instruction with named jump targets, resolved to offsets by assemble (BPF jumps only forward).
type ins struct {
	code   uint16
	k      uint32
	jt, jf string
	label  string
}

func assemble(prog []ins) []unix.SockFilter {
	at := map[string]int{}
	for i, in := range prog {
		if in.label != "" {
			at[in.label] = i
		}
	}
	out := make([]unix.SockFilter, len(prog))
	for i, in := range prog {
		f := unix.SockFilter{Code: in.code, K: in.k}
		jump := func(label string) uint8 {
			if label == "" {
				return 0
			}
			to, ok := at[label]
			if !ok || to <= i || to-i-1 > 255 {
				panic("seccomp: bad jump to " + label) // a programming error, caught by the first test run
			}
			return uint8(to - i - 1)
		}
		f.Jt, f.Jf = jump(in.jt), jump(in.jf)
		out[i] = f
	}
	return out
}

// unixSocketFilter is the program: allow everything except socket(AF_UNIX) (EACCES) and io_uring_setup (EPERM).
func unixSocketFilter() []unix.SockFilter {
	return assemble([]ins{
		{code: bpfLdAbsW, k: seccompArchOffset},
		{code: bpfJeqK, k: nativeAuditArch, jt: "native"},
		{code: bpfRet, k: seccompRetKill},
		{label: "native", code: bpfLdAbsW, k: seccompNrOffset},
		{code: bpfJsetK, k: x32SyscallBit, jf: "numbered"},
		{code: bpfRet, k: seccompRetKill},
		{label: "numbered", code: bpfJeqK, k: unix.SYS_IO_URING_SETUP, jt: "eperm"},
		{code: bpfJeqK, k: unix.SYS_SOCKET, jf: "allow"},
		{code: bpfLdAbsW, k: seccompArg0Offset},
		{code: bpfJeqK, k: afUnix, jf: "allow"},
		{code: bpfRet, k: seccompRetErrno | uint32(unix.EACCES)},
		{label: "eperm", code: bpfRet, k: seccompRetErrno | uint32(unix.EPERM)},
		{label: "allow", code: bpfRet, k: seccompRetAllow},
	})
}

// applyUnixSocketDeny installs the filter on the calling thread, which must be the thread that then execs the real command (the runner
// pins it), and the filter stays across exec. It needs no privilege once no_new_privs is set.
//
// It goes through prctl(PR_SET_SECCOMP), not the seccomp(2) syscall, on purpose: the service's own systemd unit allows
// `@system-service` and the three Landlock calls, and systemd keeps seccomp(2) in a group (@sandbox) the unit does not list, so under
// the real unit seccomp(2) answers "operation not permitted" while prctl is allowed. A container has no such filter, which is how the
// first version passed every test and still failed on vertex (2026-11-06). The cost is that the filter covers one thread, not all
// (no TSYNC), which is enough because the runner execs from this thread and exec replaces the process.
func applyUnixSocketDeny() error {
	runtime.LockOSThread() // never unlocked: the process is about to be replaced by exec
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("seccomp: set no_new_privs: %w", err)
	}
	prog := unixSocketFilter()
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&fprog)), 0, 0)
	runtime.KeepAlive(prog)
	if err != nil {
		return fmt.Errorf("seccomp: install the unix socket filter: %w", err)
	}
	return nil
}

func unixDenySupported() bool { return true }

func init() { applyUnixDeny = applyUnixSocketDeny }
