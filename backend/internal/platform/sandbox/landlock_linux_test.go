//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// Real assertions on the OS-neutral → Landlock permission mapping. Runs on any
// Linux host (CI). Confinement probe tests (a child that self-restricts and
// then attempts denied reads) are separate and must run in a subprocess,
// because landlock_restrict_self permanently confines the calling process.
func TestMapPermissions_linux(t *testing.T) {
	// Traverse = execute-only.
	if got := mapPermissions(PermTraverse); got&unix.LANDLOCK_ACCESS_FS_EXECUTE == 0 {
		t.Errorf("traverse must map to EXECUTE")
	} else if got&unix.LANDLOCK_ACCESS_FS_READ_FILE != 0 || got&unix.LANDLOCK_ACCESS_FS_WRITE_FILE != 0 {
		t.Errorf("traverse must not map read/write: %x", got)
	}

	// Read = execute + read, never write.
	if got := mapPermissions(PermRead); got&unix.LANDLOCK_ACCESS_FS_EXECUTE == 0 ||
		got&unix.LANDLOCK_ACCESS_FS_READ_FILE == 0 || got&unix.LANDLOCK_ACCESS_FS_READ_DIR == 0 {
		t.Errorf("read must map execute+read: %x", got)
	} else if got&unix.LANDLOCK_ACCESS_FS_WRITE_FILE != 0 {
		t.Errorf("read must not map write: %x", got)
	}

	// Write = create/remove/truncate set.
	if got := mapPermissions(PermWrite); got&unix.LANDLOCK_ACCESS_FS_WRITE_FILE == 0 ||
		got&unix.LANDLOCK_ACCESS_FS_MAKE_REG == 0 || got&unix.LANDLOCK_ACCESS_FS_REMOVE_FILE == 0 {
		t.Errorf("write must map create/remove/truncate: %x", got)
	}

	// Read|Write ⊇ read bits and write bits.
	rw := mapPermissions(PermRead | PermWrite)
	if rw&mapPermissions(PermRead) != mapPermissions(PermRead) {
		t.Errorf("read|write must be a superset of read")
	}
	if rw&mapPermissions(PermWrite) != mapPermissions(PermWrite) {
		t.Errorf("read|write must be a superset of write")
	}
}

// handledFSAccess must stay within what each ABI supports: TRUNCATE only exists
// from ABI v3, and nothing else in our mask is newer than v1.
func TestHandledFSAccess_linux(t *testing.T) {
	if got := handledFSAccess(1); got&unix.LANDLOCK_ACCESS_FS_TRUNCATE != 0 {
		t.Error("ABI v1 must not request TRUNCATE (added in ABI v3)")
	}
	if got := handledFSAccess(2); got&unix.LANDLOCK_ACCESS_FS_TRUNCATE != 0 {
		t.Error("ABI v2 must not request TRUNCATE (added in ABI v3)")
	}
	if got := handledFSAccess(3); got&unix.LANDLOCK_ACCESS_FS_TRUNCATE == 0 {
		t.Error("ABI v3+ must request TRUNCATE")
	}
	// Base v1 rights are present on every ABI.
	for abi := 1; abi <= 6; abi++ {
		if got := handledFSAccess(abi); got&fsRightsV1 != fsRightsV1 {
			t.Errorf("ABI v%d must handle the full v1 right set", abi)
		}
	}
}

// rulesetAttrSize mirrors the kernel struct growth: 8 bytes (FS only) through
// ABI v3, 16 with handled_access_net on v4, 24 with scoped on v5+.
func TestRulesetAttrSize_linux(t *testing.T) {
	if got := rulesetAttrSize(1); got != 8 {
		t.Errorf("ABI v1 attr size = %d, want 8", got)
	}
	if got := rulesetAttrSize(3); got != 8 {
		t.Errorf("ABI v3 attr size = %d, want 8", got)
	}
	if got := rulesetAttrSize(4); got != 16 {
		t.Errorf("ABI v4 attr size = %d, want 16", got)
	}
	if got := rulesetAttrSize(6); got != 24 {
		t.Errorf("ABI v6 attr size = %d, want 24", got)
	}
}

// The ruleset must be creatable with the exact attr size and rights the running
// ABI accepts (older kernels reject mismatched sizes with EINVAL). Creating and
// adding rules does not confine the process — only restrict_self does — so this
// runs safely in the test process.
func TestCreateRulesetMatchesRunningABI_linux(t *testing.T) {
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	fd, err := createRuleset(abi, handledFSAccess(abi), false)
	if err != nil {
		t.Fatalf("create_ruleset with ABI-appropriate size/rights failed: %v", err)
	}
	defer unix.Close(fd)
	dir := t.TempDir()
	if err := landlockAddPath(fd, dir, unix.LANDLOCK_ACCESS_FS_READ_DIR); err != nil {
		t.Fatalf("add_rule failed: %v", err)
	}
}

// Regression: a path-beneath rule may target a file or device node, but the
// kernel rejects directory-only rights (READ_DIR, MAKE_*, REMOVE_*, …) on a
// non-directory with EINVAL. Support paths like /dev/null and /etc/resolv.conf
// are files, so landlockAddPath must narrow the grant instead of failing the
// whole jail — a failure here means every Linux spawn fails closed.
func TestAddRuleFileAndDirPaths_linux(t *testing.T) {
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	fd, err := createRuleset(abi, handledFSAccess(abi), false)
	if err != nil {
		t.Fatalf("create_ruleset failed: %v", err)
	}
	defer unix.Close(fd)

	dir := t.TempDir()
	file := filepath.Join(dir, "granted.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	access := mapPermissions(PermRead|PermWrite) & handledFSAccess(abi)

	if err := landlockAddPath(fd, dir, access); err != nil {
		t.Fatalf("directory rule failed: %v", err)
	}
	if err := landlockAddPath(fd, file, access); err != nil {
		t.Fatalf("file rule failed (directory-only rights must be narrowed): %v", err)
	}
}

// Provider selection must be ABI-aware: on a Landlock-capable kernel New picks
// the Landlock provider and Effective reports it (with the network downgrade
// below ABI v4 reported honestly).
func TestNewSelectsLandlockWhenAvailable_linux(t *testing.T) {
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	p := New(Config{Filesystem: true})
	eff := p.Effective()
	if eff.Provider != "landlock" || eff.Filesystem.Mechanism != EnforcementLandlock {
		t.Fatalf("expected landlock provider on ABI %d, got %+v", abi, eff)
	}
	if abi >= 4 {
		if eff.Network.Mechanism != EnforcementLandlock || !strings.Contains(eff.Network.Reason, "network-off") {
			t.Errorf("ABI v4+ must claim TCP deny for network-off children: %+v", eff.Network)
		}
	} else if eff.Network.Mechanism != EnforcementNone {
		t.Errorf("ABI < 4 must report network as unenforced at OS level: %+v", eff.Network)
	}
}

// loaderDirs must include the standard interpreter/library roots plus the
// multiarch dir for the running machine, all absolute.
func TestLoaderDirs_linux(t *testing.T) {
	dirs := loaderDirs()
	if len(dirs) == 0 {
		t.Fatal("loaderDirs must not be empty on Linux")
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			t.Errorf("loader dir %q must be absolute", d)
		}
		if seen[d] {
			t.Errorf("duplicate loader dir %q", d)
		}
		seen[d] = true
	}
	// At least one of the generic roots must actually exist on this host —
	// otherwise the exec probe would be exercising an empty grant set.
	found := false
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("none of the loader dirs exist on this host: %v", dirs)
	}
}

// Real Landlock confinement probe (runs on Linux CI): the child applies
// ApplyLandlock for a profile that grants only the given temp dir, then must
// be able to read inside it and MUST fail to read /etc/hostname. landlock is
// permanent and self-confining, so it runs in a subprocess.
func TestLandlockConfinementProbe(t *testing.T) {
	if _, err := landlockABI(); err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	dir := t.TempDir()
	allowed := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(allowed, []byte("yes"), 0o600); err != nil {
		t.Fatal(err)
	}

	args := []string{"-test.run=TestLandlockProbeHelper"}
	if testing.CoverMode() != "" {
		// The child is coverage-instrumented too. testing's coverage teardown
		// writes intermediate data into -test.gocoverdir, and when that flag is
		// empty it creates a temp dir under TMPDIR — both outside the jail, so
		// the child exits non-zero after its confinement assertions pass. Point
		// it into the granted dir. Gated because the flag makes an
		// uninstrumented test binary exit(2).
		args = append(args, "-test.gocoverdir="+dir)
	}
	cmd := exec.Command(os.Args[0], args...)
	// GOCOVERDIR defaults outside the jail as well; keep any runtime coverage
	// emit inside the granted dir rather than failing the child's exit.
	cmd.Env = append(os.Environ(), "LLMPROXY_LANDLOCK_PROBE_DIR="+dir, "GOCOVERDIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe child failed: %v\n%s", err, out)
	}
	for _, want := range []string{"ALLOWED_OK", "DENIED_OK"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("probe output missing %s:\n%s", want, out)
		}
	}
}

func TestLandlockProbeHelper(t *testing.T) {
	dir := os.Getenv("LLMPROXY_LANDLOCK_PROBE_DIR")
	if dir == "" {
		return
	}
	// Grant: the temp dir only (write so the harness temp layout stays usable),
	// plus traversal ancestors and /dev/null for the runtime.
	p := DefaultProfile(dir, nil, []string{"/dev/null"})
	p.NeedTraverse()
	if err := ApplyLandlock(p); err != nil {
		t.Fatalf("ApplyLandlock: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(dir, "ok.txt")); err != nil {
		t.Fatalf("read allowed file failed: %v", err)
	}
	// Markers go to stdout, not t.Log: the parent parses this child's output as
	// a protocol and t.Log is suppressed unless the child is run with -test.v.
	fmt.Println("ALLOWED_OK")
	if _, err := os.ReadFile("/etc/hostname"); err == nil {
		t.Error("DENY_FAILED: read /etc/hostname succeeded under jail")
	}
	fmt.Println("DENIED_OK")
}

// TestLandlockExecProbe proves a dynamically linked binary can be exec'd under
// the jail: the child applies a realistic profile (workspace + PATH toolchain +
// loader/lib dirs + support files) and execs a shell. Exec of dynamic binaries
// requires the kernel to open the interpreter and ld.so the libraries — a read
// probe alone would not catch a missing loader grant.
func TestLandlockExecProbe(t *testing.T) {
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("Landlock unavailable: %v", err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh on PATH: %v", err)
	}
	workspace := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockExecProbeHelper")
	cmd.Env = append(os.Environ(),
		"LLMPROXY_LANDLOCK_EXEC_WORKSPACE="+workspace,
		"LLMPROXY_LANDLOCK_EXEC_SH="+sh,
		"LLMPROXY_LANDLOCK_EXEC_ABI="+fmt.Sprint(abi),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec probe child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "EXEC_OK") {
		t.Errorf("dynamic exec under jail failed:\n%s", out)
	}
}

func TestLandlockExecProbeHelper(t *testing.T) {
	workspace := os.Getenv("LLMPROXY_LANDLOCK_EXEC_WORKSPACE")
	sh := os.Getenv("LLMPROXY_LANDLOCK_EXEC_SH")
	if workspace == "" || sh == "" {
		return
	}
	abi, err := strconv.Atoi(os.Getenv("LLMPROXY_LANDLOCK_EXEC_ABI"))
	if err != nil {
		t.Fatalf("bad ABI env: %v", err)
	}
	// PATH dirs + loader dirs + support files mirror what Wrap grants; the
	// workspace gets rw.
	support := linuxSupportPaths
	support = append(support, loaderDirs()...)
	p := DefaultProfile(workspace, []string{filepath.Dir(sh)}, support)
	p.NeedTraverse()
	if err := applyLandlock(p, landlockOptions{denyNetwork: abi >= 4}); err != nil {
		t.Fatalf("ApplyLandlock: %v", err)
	}
	if err := syscall.Exec(sh, []string{"sh", "-c", "echo EXEC_OK"}, os.Environ()); err != nil {
		t.Fatalf("exec under jail failed: %v", err)
	}
}
