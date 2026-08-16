// Package paths is the single source of truth for on-disk locations. It
// resolves a single root directory (config, credentials, and runtime state all
// under one tree), creates/seeds a valid userdata directory from nothing, and
// exposes typed accessors so no component derives a managed location from a
// bare root path.
//
// Resolution precedence (highest → lowest):
//
//	explicit --data <dir>   single root for everything (resolved against CWD).
//	LLM_PROXY_HOME=<dir>    single root.
//	~/.config/llm-proxy                                  (macOS AND Linux).
//
// NOTE: this replaces the earlier two-root XDG layout (ConfigDir + DataDir).
// All files — settings.yml, registry.json, secrets.json, master.key,
// orchestrator.db, templates/, meta/, runs/, logs/ — live under one root.
// 0700 directory mode and 0600 secret-file modes provide security at rest.
package paths

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"llm-proxy/internal/platform/secretcrypto"
	"llm-proxy/models"
)

// Environment variable names consumed by Resolve/LoadMasterKey.
const (
	EnvHome      = "LLM_PROXY_HOME" // single root for everything
	EnvMasterKey = "LLM_MASTER_KEY"

	appDirName     = "llm-proxy"
	rootDirPerm    = 0o700
	secretFilePerm = 0o600

	masterKeyFilename     = "master.key"
	masterKeyHashFilename = "master.key.hash"
	databaseFilename      = "orchestrator.db"
)

// Paths holds the single resolved root. ConfigDir and DataDir are always the
// same directory — keeping both fields avoids a complete rename of every
// accessor method. Use the typed accessors, never bare path joins.
type Paths struct {
	ConfigDir string
	DataDir   string
}

// Resolve resolves the single root directory per the precedence chain above,
// creating it at 0700 and fail-fast probing that it is a real, writable
// directory (not a symlink or a regular file). The probe runs exactly once per
// Resolve call, not per lookup. ConfigDir and DataDir both equal the root.
func Resolve(explicitRootDir string) (Paths, error) {
	var root string
	var err error

	switch {
	case explicitRootDir != "":
		root, err = filepath.Abs(explicitRootDir)
		if err != nil {
			return Paths{}, fmt.Errorf("resolve --data %q: %w", explicitRootDir, err)
		}
	case os.Getenv(EnvHome) != "":
		root, err = filepath.Abs(os.Getenv(EnvHome))
		if err != nil {
			return Paths{}, fmt.Errorf("resolve LLM_PROXY_HOME %q: %w", os.Getenv(EnvHome), err)
		}
	default:
		home, herr := os.UserHomeDir()
		if herr != nil {
			return Paths{}, fmt.Errorf("resolve home dir: %w", herr)
		}
		root = filepath.Join(home, ".config", appDirName)
	}

	if err := checkRoot(root); err != nil {
		return Paths{}, fmt.Errorf("data root: %w", err)
	}
	return Paths{ConfigDir: root, DataDir: root}, nil
}

// EnforcePermissions applies the startup permission policy (S3 / Phase 11) once
// the directory is seeded. Sensitive files (master.key, master.key.hash,
// secrets.json, settings.yml, registry.json) must not be group- or
// world-readable; an unsafe mode is a fatal startup error because these gate
// ciphertext and provider credentials. Root dirs are already tightened to 0700
// by checkRoot.
func (p Paths) EnforcePermissions() error {
	sensitive := []string{
		p.MasterKeyFile(),
		p.MasterKeyHashFile(),
		p.SecretsFile(),
		p.ConfigFile(),
		p.RegistryFile(),
	}
	for _, f := range sensitive {
		fi, err := os.Lstat(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue // not yet seeded; SeedDefaults will create with 0600
			}
			return fmt.Errorf("stat %s: %w", f, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("permission policy: %s is a symlink; symlinked sensitive files are rejected", f)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("permission policy: %s is group/world-accessible (mode %o); must be 0600", f, fi.Mode().Perm())
		}
	}
	return nil
}

func checkRoot(dir string) error {
	fi, err := os.Lstat(dir)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; symlinked config/data roots are rejected", dir)
		}
		if !fi.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", dir)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(dir, rootDirPerm); err != nil {
				return fmt.Errorf("tighten permissions on %s: %w", dir, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dir, err)
	} else {
		if err := os.MkdirAll(dir, rootDirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := os.Chmod(dir, rootDirPerm); err != nil {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}

	probe, err := os.CreateTemp(dir, ".write-probe-*.tmp")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// --- Typed path accessors -------------------------------------------------

// ConfigFile returns the merged operator+user configuration document
// (settings.yml) in the config root.
func (p Paths) ConfigFile() string {
	return filepath.Join(p.ConfigDir, models.SettingsFilename)
}

// RegistryFile returns the dynamic-state registry document (registry.json).
func (p Paths) RegistryFile() string {
	return filepath.Join(p.ConfigDir, models.RegistryFilename)
}

// MasterKeyFile returns the hex-encoded AES-256 master key file.
func (p Paths) MasterKeyFile() string {
	return filepath.Join(p.ConfigDir, masterKeyFilename)
}

// MasterKeyHashFile returns the SHA-256 integrity hash of the master key.
func (p Paths) MasterKeyHashFile() string {
	return filepath.Join(p.ConfigDir, masterKeyHashFilename)
}

// SecretsFile returns the encrypted secrets payload (secrets.json), co-located
// with the master key in ConfigDir so all credential artifacts live in one place.
func (p Paths) SecretsFile() string {
	return filepath.Join(p.ConfigDir, models.SecretsFilename)
}

// DatabaseFile returns the orchestrator SQLite database path.
func (p Paths) DatabaseFile() string {
	return filepath.Join(p.DataDir, databaseFilename)
}

// TemplatesDir returns the task-template library directory.
func (p Paths) TemplatesDir() string {
	return filepath.Join(p.DataDir, "templates")
}

// MetadataDir returns the per-workspace metadata root.
func (p Paths) MetadataDir() string {
	return filepath.Join(p.DataDir, "meta")
}

// RunsDir returns the automation/recording runs root.
func (p Paths) RunsDir() string {
	return filepath.Join(p.DataDir, "runs")
}

// LogsDir returns the application log directory.
func (p Paths) LogsDir() string {
	return filepath.Join(p.DataDir, "logs")
}

// --- First-run seeding ----------------------------------------------------

// SeedDefaults creates any missing default file under the resolved roots. It is
// idempotent and never overwrites an existing file. After a successful call the
// directory is a valid, loadable userdata directory: settings.yml, registry.json,
// master.key (+hash) and an encrypted-empty secrets.json all exist.
func (p Paths) SeedDefaults() error {
	if err := p.seedFile(p.ConfigFile(), defaultSettingsYAML, secretFilePerm); err != nil {
		return err
	}
	if err := p.seedFile(p.RegistryFile(), defaultRegistryJSON, secretFilePerm); err != nil {
		return err
	}
	if err := p.seedMasterKey(); err != nil {
		return err
	}
	if err := p.seedSecrets(); err != nil {
		return err
	}
	return nil
}

// RegenerateMasterKey creates a brand-new master.key + master.key.hash via O_EXCL
// at the receiver's ConfigDir and returns the new 32-byte key. It is used by the
// factory-reset flow (file-managed key mode). If LLM_MASTER_KEY is set the
// effective key is environment-managed and this returns an error: the caller must
// reuse the environment key instead of regenerating.
func (p Paths) RegenerateMasterKey() ([]byte, error) {
	if os.Getenv(EnvMasterKey) != "" {
		return nil, fmt.Errorf("master key is environment-managed (LLM_MASTER_KEY); cannot regenerate")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	if err := writeExclusive(p.MasterKeyFile(), []byte(hex.EncodeToString(key)), secretFilePerm); err != nil {
		return nil, fmt.Errorf("write master key: %w", err)
	}
	if err := writeExclusive(p.MasterKeyHashFile(), []byte(hex.EncodeToString(sha256Hash(key))), secretFilePerm); err != nil {
		return nil, fmt.Errorf("write master key hash: %w", err)
	}
	return key, nil
}

// WriteSecretsWithKey encrypts the provided secret map with key and writes
// secrets.json at the receiver's ConfigDir. Used by factory-reset to build the
// replacement set under a (possibly new) master key.
func (p Paths) WriteSecretsWithKey(key []byte, data map[string][]models.SecretEntry) error {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}
	cipher, nonce, err := secretcrypto.EncryptAES(key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt secrets: %w", err)
	}
	payload, err := json.MarshalIndent(models.EncryptedSecretData{
		Version:   models.SecretVersionCurrent,
		Ciphertext: cipher,
		Nonce:     nonce,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secrets payload: %w", err)
	}
	return writeExclusive(p.SecretsFile(), payload, secretFilePerm)
}

// WriteSettingsFile writes a marshalled AppConfig to the given path with 0600.
func (p Paths) WriteSettingsFile(path string, cfg models.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return writeExclusive(path, data, secretFilePerm)
}

// WriteRegistryFile writes a marshalled RegistryData to the given path with 0600.
func (p Paths) WriteRegistryFile(path string, reg models.RegistryData) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return writeExclusive(path, data, secretFilePerm)
}

func (p Paths) seedFile(path string, content func() ([]byte, error), perm os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	data, err := content()
	if err != nil {
		return fmt.Errorf("build default %s: %w", filepath.Base(path), err)
	}
	if err := writeExclusive(path, data, perm); err != nil {
		return fmt.Errorf("seed %s: %w", path, err)
	}
	return nil
}

// seedMasterKey creates master.key + master.key.hash via O_EXCL. It never
// overwrites an existing key and skips file creation entirely when
// LLM_MASTER_KEY is set (the environment key is authoritative).
func (p Paths) seedMasterKey() error {
	if os.Getenv(EnvMasterKey) != "" {
		return nil
	}

	keyFile := p.MasterKeyFile()
	hashFile := p.MasterKeyHashFile()

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return fmt.Errorf("generate master key: %w", err)
		}
		if err := writeExclusive(keyFile, []byte(hex.EncodeToString(key)), secretFilePerm); err != nil {
			return fmt.Errorf("seed master key: %w", err)
		}
		if err := writeExclusive(hashFile, []byte(hex.EncodeToString(sha256Hash(key))), secretFilePerm); err != nil {
			return fmt.Errorf("seed master key hash: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("stat master key: %w", err)
	}

	// Key exists: backfill a missing hash so the key stays verifiable.
	if _, err := os.Stat(hashFile); os.IsNotExist(err) {
		key, err := loadMasterKeyFile(keyFile)
		if err != nil {
			return fmt.Errorf("load existing master key: %w", err)
		}
		if err := writeExclusive(hashFile, []byte(hex.EncodeToString(sha256Hash(key))), secretFilePerm); err != nil {
			return fmt.Errorf("seed master key hash: %w", err)
		}
	}
	return nil
}

// seedSecrets writes an encrypted-empty secrets.json payload using the
// effective master key so a fresh install has a decryptable (empty) credential
// store rather than a missing file.
func (p Paths) seedSecrets() error {
	secretsFile := p.SecretsFile()
	if _, err := os.Stat(secretsFile); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", secretsFile, err)
	}

	key, err := p.LoadMasterKey()
	if err != nil {
		return fmt.Errorf("seed secrets: %w", err)
	}

	empty := make(map[string][]models.SecretEntry)
	plaintext, err := json.Marshal(empty)
	if err != nil {
		return fmt.Errorf("seed secrets marshal: %w", err)
	}
	cipher, nonce, err := secretcrypto.EncryptAES(key, plaintext)
	if err != nil {
		return fmt.Errorf("seed secrets encrypt: %w", err)
	}
	payload, err := json.MarshalIndent(models.EncryptedSecretData{
		Version:    models.SecretVersionCurrent,
		Ciphertext: cipher,
		Nonce:      nonce,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("seed secrets marshal payload: %w", err)
	}
	return writeExclusive(secretsFile, payload, secretFilePerm)
}

func defaultSettingsYAML() ([]byte, error) {
	return yaml.Marshal(models.DefaultAppConfig())
}

func defaultRegistryJSON() ([]byte, error) {
	return json.MarshalIndent(models.RegistryData{
		Providers: make(map[string]models.ProviderRegistryEntry),
		Catalogue: []models.ModelRegistryEntry{},
	}, "", "  ")
}

// LoadMasterKey returns the effective AES-256 master key: the LLM_MASTER_KEY
// env var when set (and valid), otherwise the hex-encoded key file with its
// integrity hash verified. Failure is loud — a missing or corrupt key is never
// silently replaced by a nil key.
func (p Paths) LoadMasterKey() ([]byte, error) {
	if keyHex := os.Getenv(EnvMasterKey); keyHex != "" {
		key, err := hex.DecodeString(keyHex)
		if err == nil && len(key) == 32 {
			return key, nil
		}
		return nil, fmt.Errorf("LLM_MASTER_KEY is set but is not a valid 64-char hex-encoded 32-byte key")
	}

	key, err := loadMasterKeyFile(p.MasterKeyFile())
	if err != nil {
		return nil, err
	}

	hashFile := p.MasterKeyHashFile()
	hashData, err := os.ReadFile(hashFile)
	if err == nil {
		expected := hex.EncodeToString(sha256Hash(key))
		if string(hashData) != expected {
			return nil, fmt.Errorf("master key integrity check failed: hash mismatch at %s", hashFile)
		}
	} else if os.IsNotExist(err) {
		_ = os.WriteFile(hashFile, []byte(hex.EncodeToString(sha256Hash(key))), secretFilePerm)
	} else {
		return nil, fmt.Errorf("read master key hash: %w", err)
	}
	return key, nil
}

func loadMasterKeyFile(keyFile string) ([]byte, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key, err := hex.DecodeString(string(data))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("master key file %s is corrupted or invalid (want 64-char hex of a 32-byte key)", keyFile)
	}
	return key, nil
}

// writeExclusive writes data to path with O_EXCL so concurrent seeding can
// never race two writers; an existing file is left untouched.
func writeExclusive(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func sha256Hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
