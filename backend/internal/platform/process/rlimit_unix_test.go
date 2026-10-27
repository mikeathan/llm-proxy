//go:build darwin || linux

package process

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// Measured-OS behavior (plan §3/D5): FSIZE/NPROC/CPU are settable on Darwin and
// Linux; RLIMIT_AS must be SKIPPED on Darwin (never an error) so Effective can
// honestly report memory as not kernel-enforced there. The setter is faked: the
// real one would cap the test process itself, and a 2 GiB RLIMIT_AS makes the
// race detector die with out-of-memory on Linux.
func TestApplyLimits_ASBehavior(t *testing.T) {
	limits := ChildLimits{MaxFileSizeBytes: 1 << 30, MaxProcesses: 512, MaxCPUSeconds: 3600, AddressSpaceMB: 2048}

	t.Run("every limit settable", func(t *testing.T) {
		var got []int
		applied, skipped, err := applyLimits(limits, func(res int, _ uint64) error {
			got = append(got, res)
			return nil
		})
		if err != nil || len(skipped) != 0 {
			t.Fatalf("err=%v skipped=%v", err, skipped)
		}
		want := []string{"file-size", "processes", "cpu-seconds", "address-space"}
		if len(applied) != len(want) || len(got) != len(want) {
			t.Fatalf("applied=%v setter calls=%v, want %v", applied, got, want)
		}
		for i := range want {
			if applied[i] != want[i] {
				t.Errorf("applied[%d] = %q, want %q", i, applied[i], want[i])
			}
		}
	})

	t.Run("address space refused is skipped, not an error", func(t *testing.T) {
		applied, skipped, err := applyLimits(limits, func(res int, _ uint64) error {
			if res == unix.RLIMIT_AS {
				return unix.EINVAL
			}
			return nil
		})
		if err != nil {
			t.Fatalf("a refused RLIMIT_AS must not be an error: %v", err)
		}
		if len(applied) != 3 {
			t.Errorf("the other three limits still apply: %v", applied)
		}
		if len(skipped) != 1 || skipped[0] != "memory (RLIMIT_AS unsupported on this OS)" {
			t.Errorf("skip reason: %v", skipped)
		}
	})

	t.Run("another failure is an error", func(t *testing.T) {
		_, _, err := applyLimits(limits, func(res int, _ uint64) error {
			if res == unix.RLIMIT_NPROC {
				return unix.EPERM
			}
			return nil
		})
		if !errors.Is(err, unix.EPERM) {
			t.Errorf("err = %v, want EPERM wrapped", err)
		}
	})
}
