//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"llm-proxy/internal/platform/units"
)

func init() {
	applyRules = func(encoded string, denyNet bool) error {
		p, err := DecodeProfile(encoded)
		if err != nil {
			return err
		}
		return applyLandlock(p, landlockOptions{denyNetwork: denyNet})
	}
}

// DecodeProfile parses EncodeProfile's "perm path" lines.
func DecodeProfile(encoded string) (Profile, error) {
	p := Profile{}
	for _, line := range strings.Split(encoded, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		permStr, path, ok := strings.Cut(line, " ")
		if !ok {
			return p, fmt.Errorf("malformed rule line %q", line)
		}
		n, err := strconv.Atoi(permStr)
		if err != nil {
			return p, fmt.Errorf("malformed permission %q: %w", permStr, err)
		}
		var perm FSPermission
		if n&1 != 0 {
			perm |= PermRead
		}
		if n&2 != 0 {
			perm |= PermWrite
		}
		if n&4 != 0 {
			perm |= PermTraverse
		}
		p.Rules = append(p.Rules, Rule{Path: filepath.Clean(strings.TrimSpace(path)), Perm: perm})
	}
	return p, nil
}

// linuxSupportPaths are read-only files/dirs every jailed child needs
// (git/curl TLS, resolv, /dev nodes, Go runtime /proc reads, ld.so cache).
var linuxSupportPaths = []string{
	"/etc/ssl/certs", "/etc/ssl", "/etc/resolv.conf", "/etc/hosts",
	"/etc/ld.so.cache", "/dev/null", "/dev/zero", "/dev/random", "/dev/urandom",
	"/proc",
}

// loaderDirs returns read-only grants for the dynamic loader and shared-library
// dirs. Executing a dynamically linked binary makes the kernel (and ld.so)
// open the interpreter and its libraries, which are NOT under the PATH
// toolchain dirs — a jail that grants only PATH dirs cannot exec bash/node/git.
// The dirs below are granted read+traverse (ro), which is coarse but contains
// only system libraries/helpers, never operator secrets.
func loaderDirs() []string {
	dirs := []string{"/lib", "/lib64", "/usr/lib", "/usr/lib64", "/usr/libexec"}
	if t := multiarchTriplet(); t != "" {
		dirs = append(dirs, "/lib/"+t, "/usr/lib/"+t)
	}
	return dirs
}

// multiarchTriplet maps the running machine to its Debian multiarch directory
// name (the loader/lib layout most distros use), "" when unknown — the base
// loader dirs still apply.
func multiarchTriplet() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return ""
	}
	machine := nulString(u.Machine[:])
	switch machine {
	case "x86_64":
		return "x86_64-linux-gnu"
	case "aarch64":
		return "aarch64-linux-gnu"
	case "i686", "i386":
		return "i386-linux-gnu"
	case "armv7l", "armv6l":
		return "arm-linux-gnueabihf"
	case "ppc64le":
		return "powerpc64le-linux-gnu"
	case "s390x":
		return "s390x-linux-gnu"
	case "riscv64":
		return "riscv64-linux-gnu"
	default:
		return ""
	}
}

func nulString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// toolchainDirs extracts PATH entries from the child's prepared env (falls
// back to the process env) — the ro-exec grant set derives from the same PATH
// the shell will use, so tools resolve inside the jail.
func toolchainDirs(cmdEnv []string) []string {
	pathVal := ""
	for _, e := range cmdEnv {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			pathVal = v
			break
		}
	}
	if pathVal == "" {
		pathVal = os.Getenv("PATH")
	}
	var dirs []string
	for _, d := range filepath.SplitList(pathVal) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// landlockProvider confines a child by rewriting it through the self-exec
// runner: the child applies rlimits + ApplyLandlock and execs the real argv.
type landlockProvider struct {
	// maxFileSizeBytes arms RLIMIT_FSIZE in the child (storage backstop); 0 =
	// no per-file cap. Memory/NPROC limits are intentionally NOT set here:
	// the systemd unit's cgroup covers the service, and a per-UID NPROC cap on
	// the operator's shared uid self-DoSes (plan D5/R7).
	maxFileSizeBytes int64
}

// Wrap rewrites cmd to run through the runner with the serialized profile.
// A network-off spawn additionally requests TCP bind/connect denial when the
// kernel can express it (ABI v4+); older kernels leave network alone and
// Effective reports the gap honestly.
func (lp landlockProvider) Wrap(cmd *exec.Cmd, ws WorkspaceView) error {
	if ws.RootPath == "" {
		return nil // no workspace root → nothing to jail (non-workspace one-shot)
	}
	// Idempotent (Provider contract): a cmd already routed through the runner
	// must not be wrapped twice (double argv prefix + duplicated env markers).
	if len(cmd.Args) >= 2 && cmd.Args[1] == runnerFlag {
		return nil
	}
	abi, err := landlockABI()
	if err != nil {
		return fmt.Errorf("%w: %v", errLandlockUnsupported, err)
	}

	support := make([]string, 0, len(linuxSupportPaths)+8)
	support = append(support, linuxSupportPaths...)
	support = append(support, loaderDirs()...)
	p := DefaultProfile(ws.RootPath, toolchainDirs(cmd.Env), support)
	p.NeedTraverse()

	exe, err := os.Executable()
	if err != nil || exe == "" {
		return fmt.Errorf("landlock: cannot resolve self path: %w", err)
	}
	cmd.Path = exe
	cmd.Args = append([]string{exe, runnerFlag}, cmd.Args...)
	runnerEnv := []string{runnerEnvMarker + "=1", rulesEnvVar + "=" + EncodeProfile(p)}
	if !ws.NetworkOn && abi >= 4 {
		runnerEnv = append(runnerEnv, netDenyEnvVar+"=1")
	}
	if lp.maxFileSizeBytes > 0 {
		runnerEnv = append(runnerEnv, fsizeEnvVar+"="+strconv.FormatInt(lp.maxFileSizeBytes, 10))
	}
	appendChildEnv(cmd, runnerEnv)
	return nil
}

func (landlockProvider) Effective() EffectiveState {
	// Network enforcement is host-dependent even on Linux: ABI v4+ (kernel
	// 6.7+) can deny new TCP binds/connects for network-off children; older
	// kernels cannot express any OS network denial.
	network := SurfaceState{Mechanism: EnforcementNone, Reason: networkGapReason}
	if abi, err := landlockABI(); err == nil && abi >= 4 {
		network = SurfaceState{
			Mechanism: EnforcementLandlock,
			Reason:    "TCP bind/connect denied for network-off children (ABI v4+; includes loopback binds); UDP sendto not covered",
		}
	}
	return EffectiveState{
		Filesystem: SurfaceState{Mechanism: EnforcementLandlock},
		Network:    network,
		Provider:   "landlock",
	}
}

// newFilesystemProvider returns the Linux filesystem mechanism when the kernel
// supports it, else nil with the reason (downgrade-never-bypass).
func newFilesystemProvider(cfg Config) (Provider, string) {
	if _, err := landlockABI(); err != nil {
		return nil, fmt.Sprintf("Landlock unavailable: %v; filesystem jail not enforced", err)
	}
	return landlockProvider{maxFileSizeBytes: units.GiB(cfg.MaxStorageGB)}, ""
}
