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

// applyUnixSocketDeny installs the filter on every thread of this process (so the thread that execs the real command carries it) and
// keeps it across exec. It needs no privilege once no_new_privs is set.
func applyUnixSocketDeny() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("seccomp: set no_new_privs: %w", err)
	}
	prog := unixSocketFilter()
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, seccompSetModeFilter, seccompFilterFlagTsync, uintptr(unsafe.Pointer(&fprog)))
	runtime.KeepAlive(prog)
	if errno != 0 {
		return fmt.Errorf("seccomp: install the unix socket filter: %w", errno)
	}
	return nil
}

const (
	seccompSetModeFilter   = 1
	seccompFilterFlagTsync = 1
)

func unixDenySupported() bool { return true }

func init() { applyUnixDeny = applyUnixSocketDeny }
