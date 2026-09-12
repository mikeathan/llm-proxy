//go:build darwin || linux

package process

import (
	"fmt"

	"golang.org/x/sys/unix"

	"llm-proxy/internal/platform/units"
)

// ChildLimits are per-process resource caps for agent children (plan Phase 3 /
// D5). Zero values leave that limit untouched. Memory is NOT expressible here
// on every OS (Darwin cannot set RLIMIT_AS/RLIMIT_DATA — measured) and is the
// systemd cgroup's job on the Linux service; see ApplyChildLimits.
type ChildLimits struct {
	MaxFileSizeBytes int64 // RLIMIT_FSIZE (0 = untouched)
	MaxProcesses     int   // RLIMIT_NPROC (0 = untouched)
	MaxCPUSeconds    int   // RLIMIT_CPU (0 = untouched)
	// AddressSpaceMB is a LINUX-only soft/hard RLIMIT_AS cap; on Darwin it is
	// skipped (the OS refuses to set it — reported, not silently ignored).
	AddressSpaceMB int
}

// ApplyChildLimits lowers the CALLING process's soft+hard limits. Callers must
// invoke it in the child (post-fork, pre-exec) — lowering the backend's own
// limits here would starve the server. Unsupported/refused limits (Darwin
// RLIMIT_AS/RLIMIT_DATA) are reported as skipped, never as errors, so the
// honest Effective state can say "memory not kernel-enforced on Darwin".
func ApplyChildLimits(c ChildLimits) (applied []string, skipped []string, err error) {
	set := func(res int, label string, value uint64) error {
		if err := unix.Setrlimit(res, &unix.Rlimit{Cur: value, Max: value}); err != nil {
			return err
		}
		applied = append(applied, label)
		return nil
	}

	if c.MaxFileSizeBytes > 0 {
		if err := set(unix.RLIMIT_FSIZE, "file-size", uint64(c.MaxFileSizeBytes)); err != nil {
			return applied, skipped, fmt.Errorf("rlimit FSIZE: %w", err)
		}
	}
	if c.MaxProcesses > 0 {
		if err := set(unix.RLIMIT_NPROC, "processes", uint64(c.MaxProcesses)); err != nil {
			return applied, skipped, fmt.Errorf("rlimit NPROC: %w", err)
		}
	}
	if c.MaxCPUSeconds > 0 {
		if err := set(unix.RLIMIT_CPU, "cpu-seconds", uint64(c.MaxCPUSeconds)); err != nil {
			return applied, skipped, fmt.Errorf("rlimit CPU: %w", err)
		}
	}
	if c.AddressSpaceMB > 0 {
		if err := set(unix.RLIMIT_AS, "address-space", uint64(units.MiB(c.AddressSpaceMB))); err != nil {
			// Darwin refuses RLIMIT_AS (EINVAL — measured); macOS memory caps are
			// not kernel-enforceable. Report as skipped, not as an error.
			if err == unix.EINVAL {
				skipped = append(skipped, "memory (RLIMIT_AS unsupported on this OS)")
				return applied, skipped, nil
			}
			return applied, skipped, fmt.Errorf("rlimit AS: %w", err)
		}
	}
	return applied, skipped, nil
}
