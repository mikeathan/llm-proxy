//go:build darwin || linux

package process

import (
	"testing"
)

// Measured-OS behavior (plan §3/D5): FSIZE/NPROC/CPU are settable on Darwin and
// Linux; RLIMIT_AS must be SKIPPED on Darwin (never an error) so Effective can
// honestly report memory as not kernel-enforced there.
func TestApplyChildLimits_ASBehavior(t *testing.T) {
	applied, skipped, err := ApplyChildLimits(ChildLimits{
		MaxFileSizeBytes: 1 << 30, // 1 GiB
		MaxProcesses:     512,
		MaxCPUSeconds:    3600,
		AddressSpaceMB:   2048,
	})
	if err != nil {
		t.Fatalf("ApplyChildLimits: %v", err)
	}
	joined := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, s := range list {
			m[s] = true
		}
		return m
	}
	a := joined(applied)
	for _, want := range []string{"file-size", "processes", "cpu-seconds"} {
		if !a[want] {
			t.Errorf("expected %q applied, got %v", want, applied)
		}
	}
	// The AS cap was either applied (Linux) or explicitly skipped (Darwin) —
	// it must never silently vanish.
	if len(skipped) == 0 && !a["address-space"] {
		t.Errorf("AS cap neither applied nor reported skipped: applied=%v skipped=%v", applied, skipped)
	}
	if len(skipped) > 0 && !joined(skipped)["memory (RLIMIT_AS unsupported on this OS)"] {
		t.Errorf("unexpected skip reason: %v", skipped)
	}
}
