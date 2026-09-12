//go:build !linux

package sandbox

// newFilesystemProvider reports no OS filesystem mechanism on this build.
// macOS containment is uid-first (dedicated-user deployment, plan D4); Seatbelt
// can only be built where the Sandbox framework is available. The no-op
// provider reports this honestly (downgrade-never-bypass).
func newFilesystemProvider(_ Config) (Provider, string) {
	return nil, "no filesystem jail mechanism on this build (macOS: uid-first/launchd containment; seatbelt helper requires the Sandbox framework)"
}
