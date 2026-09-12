//go:build linux

// Landlock enforcement (agent-os-sandboxing plan Phase 2b): installs a
// deny-by-default filesystem ruleset on the calling process via the landlock
// syscalls (x/sys exposes the constants and syscall numbers; wrappers are
// hand-rolled here because the ruleset attr size must match the running
// kernel's ABI — see createRuleset).
//
// Capability detection is a real syscall probe (landlockABI), never a
// release-string guess: kernels where Landlock is compiled out, disabled at
// boot, or older than 5.13 are all reported here. Rulesets request only the
// rights the detected ABI supports and are created with the exact attr size
// that ABI expects — older kernels reject unknown rights and mismatched struct
// sizes (EINVAL), which a "kernel >= 5.13" check would not catch.
//
// Landlock is self-confining (landlock_restrict_self), so ApplyLandlock must
// run in the child process itself — the sandbox-runner re-execs the real argv
// after applying the rules. On ABI v4+ (kernel >= 6.7) a ruleset option denies
// new TCP binds/connects for network-off shells (plan D8); older kernels
// cannot express OS network denial and Effective reports the downgrade.
package sandbox

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const landlockRulePathBeneath = uint64(unix.LANDLOCK_RULE_PATH_BENEATH)

// fsRightsV1 is the set of Landlock FS rights available since ABI v1 (kernel
// 5.13): execute/read/write/create/remove on a granted subtree. Later ABIs add
// REFER (v2, 5.19), TRUNCATE (v3, 6.2), and network rights (v4, 6.7).
//
// REFER is deliberately not handled: renaming inside the workspace is governed
// by the REMOVE/MAKE rights of the workspace grant, and unhandled-REFER keeps
// the kernel's compatibility behaviour outside the workspace without an extra
// right this package never grants.
const fsRightsV1 = uint64(
	unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM,
)

// netRights are the Landlock network rights this package uses to deny new
// TCP bind/connect for network-off processes (ABI v4+, kernel >= 6.7).
const netRights = uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)

// fileRightsMask are the rights Landlock accepts on a non-directory path.
// A path-beneath rule may target a file or device node, but the kernel rejects
// the directory-only rights (READ_DIR, REMOVE_*, MAKE_*) with EINVAL when
// parent_fd refers to anything that is not a directory.
const fileRightsMask = uint64(
	unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE)

// handledFSAccess returns the FS rights this package enforces that the running
// ABI supports. TRUNCATE only exists from ABI v3 (kernel 6.2); requesting it on
// older kernels makes create_ruleset fail (EINVAL).
func handledFSAccess(abi int) uint64 {
	rights := fsRightsV1
	if abi >= 3 {
		rights |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return rights
}

// rulesetAttrSize is the size of struct landlock_ruleset_attr on the running
// kernel: 8 bytes (handled_access_fs) through ABI v3, 16 with
// handled_access_net on ABI v4, 24 with the scoped field on ABI v5+ (kernel
// 6.10+). The kernel copies exactly its own struct size from the caller's
// buffer, so a mismatched size fails (EINVAL) on kernels that predate a newer
// userspace.
func rulesetAttrSize(abi int) uintptr {
	switch {
	case abi >= 5:
		return 24
	case abi >= 4:
		return 16
	default:
		return 8
	}
}

var (
	landlockOnce sync.Once
	landlockABIV int
	landlockErr  error
)

// landlockABI reports the kernel Landlock ABI version (>= 1) and whether the
// mechanism is usable at all. It probes landlock_create_ruleset with the
// VERSION flag — the documented capability check. The result is cached: the
// kernel does not change under a process. EOPNOTSUPP means compiled out or
// disabled at boot; ENOSYS means the syscall does not exist (kernel < 5.13).
func landlockABI() (int, error) {
	landlockOnce.Do(func() {
		abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
		if errno != 0 {
			landlockErr = errno
			return
		}
		if abi == 0 { // ABI 0 does not exist — treat as unavailable.
			landlockErr = syscall.EOPNOTSUPP
			return
		}
		landlockABIV = int(abi)
	})
	return landlockABIV, landlockErr
}

// mapPermissions translates the OS-neutral grant model to Landlock rights.
// PermRead grants execute (binary + traversal) and read; PermWrite grants
// create/remove/truncate; PermTraverse is execute-only. TRUNCATE is only
// meaningful on ABI v3+ — callers mask the result to the handled set.
func mapPermissions(perm FSPermission) uint64 {
	var a uint64
	if perm&(PermRead|PermTraverse) != 0 {
		a |= unix.LANDLOCK_ACCESS_FS_EXECUTE
	}
	if perm&PermRead != 0 {
		a |= unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	}
	if perm&PermWrite != 0 {
		a |= unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
			unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
			unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
			unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_SYM | unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return a
}

// createRuleset creates a ruleset handling the given FS rights (and TCP
// bind/connect rights when denyNetwork is requested). The caller owns the
// returned fd and must close it. Creating a ruleset does not confine anything —
// confinement begins at landlockRestrictSelf.
func createRuleset(abi int, handledFS uint64, denyNetwork bool) (int, error) {
	var handledNet uint64
	if denyNetwork {
		handledNet = netRights
	}
	var raw [24]byte // zero-filled; the kernel reads only rulesetAttrSize(abi)
	*(*uint64)(unsafe.Pointer(&raw[0])) = handledFS
	if rulesetAttrSize(abi) >= 16 {
		*(*uint64)(unsafe.Pointer(&raw[8])) = handledNet
	}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&raw[0])), rulesetAttrSize(abi), 0)
	if errno != 0 {
		return -1, fmt.Errorf("landlock: create ruleset: %w", errno)
	}
	return int(fd), nil
}

// ApplyLandlock restricts the CALLING process to the profile's grants (and,
// when opts.denyNetwork is set, denies new TCP binds/connects — plan D8). It
// must run in the child (self-confining); after success the process cannot read
// or write anything not granted. Requires PR_SET_NO_NEW_PRIVS (unprivileged
// restrict_self). Returns an error (typed as landlockUnsupported when the
// kernel lacks Landlock or cannot express the requested option) without having
// restricted the process.
func ApplyLandlock(p Profile) error {
	return applyLandlock(p, landlockOptions{})
}

// applyLandlockOptions carries the confinement options beyond the FS profile.
type landlockOptions struct {
	// denyNetwork denies new TCP binds/connects (Landlock ABI v4+, kernel
	// >= 6.7). Only expressible when the ABI provides network rights; on older
	// kernels a request to deny network is an error so a caller cannot
	// accidentally believe it is enforced.
	denyNetwork bool
}

func applyLandlock(p Profile, opts landlockOptions) error {
	abi, err := landlockABI()
	if err != nil {
		return fmt.Errorf("%w: %v", errLandlockUnsupported, err)
	}
	if opts.denyNetwork && abi < 4 {
		return fmt.Errorf("%w: network denial needs Landlock ABI v4 (kernel >= 6.7), have ABI v%d", errLandlockUnsupported, abi)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("landlock: set no_new_privs: %w", err)
	}

	ruleset, err := createRuleset(abi, handledFSAccess(abi), opts.denyNetwork)
	if err != nil {
		return err
	}
	defer unix.Close(ruleset)

	handled := handledFSAccess(abi)
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		// Never request a right the ruleset does not handle (older kernels
		// reject add_rule with EINVAL): mask to the handled set.
		if err := landlockAddPath(ruleset, r.Path, mapPermissions(r.Perm)&handled); err != nil {
			return err
		}
	}

	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(ruleset), 0, 0); errno != 0 {
		return fmt.Errorf("landlock: restrict self: %w", errno)
	}
	return nil
}

func landlockAddPath(ruleset int, path string, access uint64) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		// A missing grant path must not fail the whole jail (e.g. an optional
		// toolchain dir absent on this host): skip it.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("landlock: open %s: %w", path, err)
	}
	defer unix.Close(fd)

	// Path-beneath rules support files as well as directories, but the kernel
	// rejects the directory-only rights on a non-directory with EINVAL. Most
	// grants are directories (workspace, toolchain, loader); the support paths
	// (/dev/null, /etc/resolv.conf, …) are files, so narrow those to the
	// file-compatible set and skip a rule that would grant nothing.
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("landlock: stat %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		if access &= fileRightsMask; access == 0 {
			return nil
		}
	}

	beneath := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset),
		uintptr(landlockRulePathBeneath), uintptr(unsafe.Pointer(&beneath))); errno != 0 {
		return fmt.Errorf("landlock: add rule for %s: %w", path, errno)
	}
	return nil
}

// errLandlockUnsupported distinguishes "kernel can't do this" so callers can
// downgrade honestly (EffectiveState) instead of treating it as a hard error.
var errLandlockUnsupported = syscall.EOPNOTSUPP
