package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/paths"
	"llm-proxy/models"
)

// ResetResult reports what factory-reset did, including whether the master key
// was regenerated (false in environment-managed key mode).
type ResetResult struct {
	KeyRegenerated bool `json:"key_regenerated"`
	KeyExternallyManaged bool `json:"key_externally_managed"`
}

// FactoryReset performs the staged, failure-safe reset described in Phase 10 of
// the XDG relocation plan. It builds the complete replacement set (default
// settings.yml, default registry.json, a freshly generated master key + hash in
// file-managed mode, and an encrypted-empty secrets.json under the effective
// key) in a private temp dir, validates it, then atomically swaps the active
// files into ConfigDir/DataDir. orchestrator.db, meta/, templates/ and
// workspaces/ are deliberately left untouched.
//
// After the swap it reloads every store WITHOUT firing OnChange (so the empty
// post-reset registry/settings do not trigger a mid-reset Sync) and rebuilds the
// SecretStore around the effective key, satisfying the post-reset secret
// invariant. The caller is responsible for quiescing runtime work before calling.
func (m *DataManager) FactoryReset() (ResetResult, error) {
	res := ResetResult{}

	// 1. Build the replacement set in a private temp dir.
	stage, err := os.MkdirTemp("", "llm-proxy-reset-*")
	if err != nil {
		return res, fmt.Errorf("create reset staging dir: %w", err)
	}
	defer os.RemoveAll(stage)

	staged := paths.Paths{
		ConfigDir: stage,
		DataDir:   stage,
	}

	// 2. Effective key: reuse the environment key, or regenerate a fresh one in
	// the staging dir (file-managed mode).
	var key []byte
	if os.Getenv(paths.EnvMasterKey) != "" {
		k, lerr := paths.Paths{ConfigDir: m.paths.ConfigDir}.LoadMasterKey()
		if lerr != nil {
			return res, fmt.Errorf("load environment master key: %w", lerr)
		}
		key = k
		res.KeyExternallyManaged = true
	} else {
		k, rerr := staged.RegenerateMasterKey()
		if rerr != nil {
			return res, fmt.Errorf("regenerate master key: %w", rerr)
		}
		key = k
		res.KeyRegenerated = true
	}

	if err := staged.WriteSettingsFile(filepath.Join(stage, models.SettingsFilename), models.DefaultAppConfig()); err != nil {
		return res, fmt.Errorf("stage settings: %w", err)
	}
	if err := staged.WriteRegistryFile(filepath.Join(stage, models.RegistryFilename), models.RegistryData{
		Providers: make(map[string]models.ProviderRegistryEntry),
		Catalogue: []models.ModelRegistryEntry{},
	}); err != nil {
		return res, fmt.Errorf("stage registry: %w", err)
	}
	if err := staged.WriteSecretsWithKey(key, make(map[string][]models.SecretEntry)); err != nil {
		return res, fmt.Errorf("stage secrets: %w", err)
	}

	// 3. Validate the staged set before activation.
	if err := validateStagedSet(staged, key); err != nil {
		return res, fmt.Errorf("validate staged reset set: %w", err)
	}

	// 4. Swap into place with a recoverable backup. A failure mid-swap rolls the
	// previously backed-up set back into place so key/ciphertext never diverge.
	backup, err := os.MkdirTemp("", "llm-proxy-backup-*")
	if err != nil {
		return res, fmt.Errorf("create reset backup dir: %w", err)
	}
	defer os.RemoveAll(backup)

	targets := []swapTarget{
		{src: filepath.Join(stage, models.SettingsFilename), dst: m.paths.ConfigFile()},
		{src: filepath.Join(stage, models.RegistryFilename), dst: m.paths.RegistryFile()},
		{src: filepath.Join(stage, models.SecretsFilename), dst: m.paths.SecretsFile()},
	}
	// master.key / master.key.hash only exist in file-managed mode.
	if !res.KeyExternallyManaged {
		targets = append(targets,
			swapTarget{src: staged.MasterKeyFile(), dst: m.paths.MasterKeyFile()},
			swapTarget{src: staged.MasterKeyHashFile(), dst: m.paths.MasterKeyHashFile()},
		)
	}

	for _, t := range targets {
		if err := backupFile(t.dst, backup); err != nil {
			return res, err
		}
	}
	for _, t := range targets {
		if err := swapFile(t.src, t.dst); err != nil {
			restoreFromBackup(backup, targets)
			return res, fmt.Errorf("swap %s: %w", t.dst, err)
		}
	}

	// 5. Reload stores without notifying runtime subscribers, then rebuild the
	// SecretStore around the effective key (Store.Load cannot swap the captured
	// key).
	if err := m.reloadQuiet(); err != nil {
		return res, fmt.Errorf("reload after reset: %w", err)
	}
	m.rebuildSecretStore(key)

	logging.Info("factory reset complete",
		"key_regenerated", res.KeyRegenerated,
		"key_externally_managed", res.KeyExternallyManaged)
	return res, nil
}

// reloadQuiet reloads every store from disk without firing OnChange.
func (m *DataManager) reloadQuiet() error {
	if err := m.appConfigStore.LoadQuiet(); err != nil {
		return err
	}
	if err := m.registryStore.LoadQuiet(); err != nil {
		return err
	}
	if err := m.encSecretStore.LoadQuiet(); err != nil {
		return err
	}
	return nil
}

// rebuildSecretStore reconstructs the SecretStore with a (possibly new) master
// key. The previous store's cached decrypted view is discarded. The swap is
// guarded by secretsMu so concurrent Secrets() reads never observe a torn store.
func (m *DataManager) rebuildSecretStore(key []byte) {
	m.secretsMu.Lock()
	defer m.secretsMu.Unlock()
	m.secretsStore = NewSecretStore(m.encSecretStore, key)
}

// validateStagedSet loads the staged settings/registry and round-trips the
// staged secrets with the effective key to prove the replacement set is usable.
func validateStagedSet(staged paths.Paths, key []byte) error {
	if _, err := os.Stat(staged.ConfigFile()); err != nil {
		return fmt.Errorf("settings missing: %w", err)
	}
	if _, err := os.Stat(staged.RegistryFile()); err != nil {
		return fmt.Errorf("registry missing: %w", err)
	}
	// Verify key/hash mutually valid (file-managed mode only).
	if _, err := staged.LoadMasterKey(); err != nil {
		// In env-managed mode LoadMasterKey returns the env key; tolerate either.
		if os.Getenv(paths.EnvMasterKey) == "" {
			return fmt.Errorf("master key invalid: %w", err)
		}
	}
	// Round-trip secrets: write a key, read it back.
	tmp := NewStore[models.EncryptedSecretData](staged.SecretsFile())
	ss := NewSecretStore(tmp, key)
	if err := ss.SetProviderKeys("probe", []models.APIKeyItem{{ID: "p1", Name: "p", Key: "sk-probe"}}); err != nil {
		return fmt.Errorf("secrets round-trip write: %w", err)
	}
	if got := ss.GetProviderKeys("probe"); len(got) != 1 || got[0].Key != "sk-probe" {
		return fmt.Errorf("secrets round-trip mismatch")
	}
	return nil
}

// ClearRuntimeData deletes only the known allowlist of runtime state derived from
// paths.Paths (never a recursive root wipe): per-workspace sessions, process.log,
// .lock under meta/, plus runs/ and logs/. orchestrator.db, meta/ structure,
// templates/ and workspaces/ are untouched. The caller must quiesce active
// sessions/runs first.
func (m *DataManager) ClearRuntimeData() error {
	targets := []string{
		m.paths.RunsDir(),
		m.paths.LogsDir(),
	}
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}

	// Per-workspace meta runtime files.
	meta := m.paths.MetadataDir()
	wsDirs, err := os.ReadDir(meta)
	if err == nil {
		for _, ws := range wsDirs {
			if !ws.IsDir() {
				continue
			}
			base := filepath.Join(meta, ws.Name())
			removeIfExists(filepath.Join(base, "sessions"))
			removeIfExists(filepath.Join(base, "process.log"))
			removeIfExists(filepath.Join(base, ".lock"))
		}
	}

	// Recreate the required empty directories (logs/ must exist for the file
	// logger; runs/ for recording).
	for _, t := range targets {
		if err := os.MkdirAll(t, 0o700); err != nil {
			return fmt.Errorf("recreate %s: %w", t, err)
		}
	}
	return nil
}

// WipeoutResult reports the two on-disk locations a full uninstall removed.
type WipeoutResult struct {
	RootDir       string `json:"root_dir"`
	WorkspacesDir string `json:"workspaces_dir"`
}

// Wipeout removes everything the service has created: the entire single root
// directory (settings, registry, secrets, master key, orchestrator db, templates,
// meta, runs, logs) and the workspaces directory. Unlike factory-reset and
// clear-runtime-data this is a full uninstall, so the caller must have already
// stopped the runtime (watcher, metrics, shell, DB) before calling. Both targets
// are validated so a misresolved path can never wipe "/", $HOME, or an ancestor
// of $HOME.
func (m *DataManager) Wipeout() (WipeoutResult, error) {
	root := m.RootDir()
	ws := m.WorkspacesDir()

	res := WipeoutResult{RootDir: root, WorkspacesDir: ws}

	if err := validateWipeTarget(root); err != nil {
		return res, fmt.Errorf("refusing to wipe data root: %w", err)
	}
	if err := validateWipeTarget(ws); err != nil {
		return res, fmt.Errorf("refusing to wipe workspaces dir: %w", err)
	}

	if err := os.RemoveAll(root); err != nil {
		return res, fmt.Errorf("wipe data root %s: %w", root, err)
	}
	// Workspaces may already be gone if it resolves under the data root; a
	// second RemoveAll on an absent path is a harmless no-op.
	if err := os.RemoveAll(ws); err != nil {
		return res, fmt.Errorf("wipe workspaces dir %s: %w", ws, err)
	}

	logging.Info("wipeout complete", "root", root, "workspaces", ws)
	return res, nil
}

// validateWipeTarget refuses to delete the filesystem root, the user's home
// directory, or any ancestor of the home directory. Wiping "/" or $HOME is never
// a legitimate uninstall target and indicates a misresolved --data/workspaces_dir.
func validateWipeTarget(p string) error {
	clean := filepath.Clean(p)
	if clean == "." || clean == string(os.PathSeparator) {
		return fmt.Errorf("%q is not a safe wipe target", p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		homeClean := filepath.Clean(home)
		if clean == homeClean {
			return fmt.Errorf("%q is the user home directory; refusing to wipe it", p)
		}
		if strings.HasPrefix(homeClean, clean+string(os.PathSeparator)) {
			return fmt.Errorf("%q is an ancestor of the home directory; refusing to wipe it", p)
		}
	}
	return nil
}

// swapTarget pairs a staged replacement file with its active destination.
type swapTarget struct{ src, dst string }

// backupFile copies src into backupDir (named by base(src)) so a failed swap
// can be rolled back. A missing src is not an error.
func backupFile(src, backupDir string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("backup %s: %w", src, err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), data, 0o600); err != nil {
		return fmt.Errorf("backup write %s: %w", src, err)
	}
	return nil
}

// swapFile atomically renames src onto dst. A failed rename aborts the reset so
// a partial swap is never reported as success.
func swapFile(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source %s missing: %w", src, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return nil
}

// restoreFromBackup rolls the backed-up files back into place after a failed
// swap. Best-effort: if a backup file is missing, that path is left as-is.
func restoreFromBackup(backupDir string, targets []swapTarget) {
	for _, t := range targets {
		src := filepath.Join(backupDir, filepath.Base(t.dst))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.Rename(src, t.dst)
	}
}

func removeIfExists(path string) {
	if _, err := os.Stat(path); err == nil {
		_ = os.RemoveAll(path)
	}
}
