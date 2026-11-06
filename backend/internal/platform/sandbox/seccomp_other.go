//go:build !(linux && (amd64 || arm64))

package sandbox

// The Unix-socket denial is a seccomp filter written for linux/amd64 and linux/arm64. Elsewhere it cannot be applied, and a provider
// reports that (Effective) instead of claiming it.
func unixDenySupported() bool { return false }
