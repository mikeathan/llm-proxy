//go:build !darwin && !linux

package process

// ApplyChildLimits is a no-op on unsupported OSes (no unix rlimits).
func ApplyChildLimits(ChildLimits) (applied []string, skipped []string, err error) {
	return nil, []string{"memory/process limits unsupported on this OS"}, nil
}
