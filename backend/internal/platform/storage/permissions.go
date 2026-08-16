package storage

import "os"

// FileClass declares the security sensitivity of an atomically-written file so
// callers never pass raw octal modes. Each class pairs a directory mode (used
// when creating/tightening the parent) with a file mode (applied to the final
// destination). Centralising the mapping here is the single source of truth for
// on-disk permission policy (CONSTITUTION III.6, storage S1/S2 findings).
//
// The classes intentionally collapse the historical scatter of 0o700/0o755
// literals into one readable table: managed state is always 0700/0600; only
// user-owned workspace content is relaxed to 0644 because the same OS user (and
// the agent's own file tools) reads it.
type FileClass int

const (
	// ClassSecret is for key material and ciphertext (master.key, secrets.json).
	// Directory 0700, file 0600 — the tightest tier; a leak here is fatal.
	ClassSecret FileClass = iota

	// ClassConfig is for hand-edited or operator configuration that may carry
	// provider endpoints/credentials (settings.yml, registry.json, host.json,
	// config.json). Directory 0700, file 0600.
	ClassConfig

	// ClassData is for application-internal state (orchestrator.db, templates,
	// derived caches). Directory 0700, file 0600.
	ClassData

	// ClassUserContent is for per-workspace runtime files (state.json,
	// sessions, heartbeat, task files, edited documents). Owned by the same OS
	// user that reads them; directory 0700, file 0644.
	ClassUserContent
)

// fileClassPolicy is the single source of truth mapping each FileClass to its
// directory and file permissions. It is intentionally a plain lookup table so
// the policy is auditable in one place.
var fileClassPolicy = map[FileClass]struct {
	dirPerm  os.FileMode
	filePerm os.FileMode
}{
	ClassSecret:      {dirPerm: 0o700, filePerm: 0o600},
	ClassConfig:      {dirPerm: 0o700, filePerm: 0o600},
	ClassData:        {dirPerm: 0o700, filePerm: 0o600},
	ClassUserContent: {dirPerm: 0o700, filePerm: 0o644},
}

// dirMode returns the directory permission for the class.
func (c FileClass) dirMode() os.FileMode {
	return fileClassPolicy[c].dirPerm
}

// fileMode returns the final file permission for the class.
func (c FileClass) fileMode() os.FileMode {
	return fileClassPolicy[c].filePerm
}
