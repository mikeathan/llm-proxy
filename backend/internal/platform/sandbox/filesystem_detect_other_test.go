//go:build !linux

package sandbox

// filesystemMechanismAvailable reports whether this host selects a real
// filesystem mechanism (test-only seam; non-Linux has none).
func filesystemMechanismAvailable() bool { return false }
