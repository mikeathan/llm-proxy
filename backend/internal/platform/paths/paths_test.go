package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"llm-proxy/internal/platform/secretcrypto"
	"llm-proxy/models"
)

func mustHex32(t *testing.T, n int) string {
	t.Helper()
	raw := make([]byte, 32)
	copy(raw, []byte("0123456789abcdef0123456789abcdef"))
	for i := range raw {
		raw[i] ^= byte(i + n)
	}
	return hex.EncodeToString(raw)
}

// resetEnv clears every env var that influences Resolve so each test starts
// from a clean slate.
func resetEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvHome, EnvMasterKey} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", t.TempDir())
}

func TestResolve_ExplicitRoot(t *testing.T) {
	resetEnv(t)
	dir := t.TempDir()

	p, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if p.ConfigDir != abs {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, abs)
	}
	if p.DataDir != abs {
		t.Errorf("DataDir = %q, want %q", p.DataDir, abs)
	}
}

func TestResolve_ExplicitRootRelativeToCWD(t *testing.T) {
	resetEnv(t)
	p, err := Resolve("rel-data")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	cwd, _ := os.Getwd()
	want := filepath.Join(cwd, "rel-data")
	if p.DataDir != want || p.ConfigDir != want {
		t.Errorf("ConfigDir/DataDir = %q, want %q", p.DataDir, want)
	}
}

func TestResolve_LLMProxyHome(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	t.Setenv(EnvHome, home)

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ConfigDir != home {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, home)
	}
	if p.DataDir != home {
		t.Errorf("DataDir = %q, want %q", p.DataDir, home)
	}
}

func TestResolve_Defaults(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(home, ".config", "llm-proxy")
	if p.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, want)
	}
	if p.DataDir != want {
		t.Errorf("DataDir = %q, want %q", p.DataDir, want)
	}
}

func TestResolve_ExplicitRootBeatsEnv(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	explicit := t.TempDir()
	t.Setenv(EnvHome, home)

	p, err := Resolve(explicit)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.DataDir != explicit || p.ConfigDir != explicit {
		t.Errorf("ConfigDir/DataDir = %q/%q, want explicit %q", p.ConfigDir, p.DataDir, explicit)
	}
}

func TestResolve_CreatesDir0700(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	t.Setenv(EnvHome, home)

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	fi, err := os.Stat(p.DataDir)
	if err != nil {
		t.Fatalf("stat %s: %v", p.DataDir, err)
	}
	if !fi.IsDir() {
		t.Errorf("%s is not a dir", p.DataDir)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("%s mode = %o, want 0700", p.DataDir, fi.Mode().Perm())
	}
}

func TestResolve_FailsWhenRootIsRegularFile(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	root := filepath.Join(home, ".config", "llm-proxy")
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	if _, err := Resolve(""); err == nil {
		t.Fatal("expected error when root is a regular file")
	} else if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected not-a-directory error, got: %v", err)
	}
}

func TestResolve_FailsWhenRootIsSymlink(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	real := filepath.Join(home, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, ".config", "llm-proxy")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	if _, err := Resolve(""); err == nil {
		t.Fatal("expected error when root is a symlink")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}
}

func TestResolve_FailsWhenRootUnwritable(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	root := filepath.Join(home, ".config", "llm-proxy")
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	t.Setenv("HOME", home)

	if _, err := Resolve(""); err == nil {
		t.Fatal("expected error when root is not writable")
	} else if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("expected not-writable error, got: %v", err)
	}
}

func TestResolve_TightensOpenExistingDir(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	root := filepath.Join(home, ".config", "llm-proxy")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	if _, err := Resolve(""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 0700 after tighten", fi.Mode().Perm())
	}
}

func TestSeedDefaults_EmptyHome(t *testing.T) {
	resetEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	// settings.yml loads as AppConfig with defaults applied.
	cfgData, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatalf("settings.yml missing: %v", err)
	}
	var cfg models.AppConfig
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatalf("settings.yml not loadable: %v", err)
	}
	if cfg.Server.Bind != "0.0.0.0:4001" {
		t.Errorf("default bind = %q", cfg.Server.Bind)
	}
	if !cfg.Sandboxing.Enabled {
		t.Error("default sandboxing should be enabled")
	}
	if cfg.Memory == nil || !cfg.Memory.Enabled {
		t.Error("default memory should be enabled")
	}

	// registry.json loads.
	regData, err := os.ReadFile(p.RegistryFile())
	if err != nil {
		t.Fatalf("registry.json missing: %v", err)
	}
	var reg models.RegistryData
	if err := json.Unmarshal(regData, &reg); err != nil {
		t.Fatalf("registry.json not loadable: %v", err)
	}
	if reg.Providers == nil {
		t.Error("registry providers should be non-nil map")
	}

	// master.key is a valid 64-char hex decoding to 32 bytes.
	keyData, err := os.ReadFile(p.MasterKeyFile())
	if err != nil {
		t.Fatalf("master.key missing: %v", err)
	}
	if len(keyData) != 64 {
		t.Errorf("master.key is %d bytes, want 64-char hex", len(keyData))
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil || len(key) != 32 {
		t.Fatalf("master.key does not decode to 32 bytes: %v", err)
	}

	// .hash verifies.
	hashData, err := os.ReadFile(p.MasterKeyHashFile())
	if err != nil {
		t.Fatalf("master.key.hash missing: %v", err)
	}
	sum := sha256.Sum256(key)
	if strings.TrimSpace(string(hashData)) != hex.EncodeToString(sum[:]) {
		t.Error("master.key.hash does not verify against master.key")
	}

	// secrets.json round-trips: encrypted empty payload decrypts to empty map.
	secData, err := os.ReadFile(p.SecretsFile())
	if err != nil {
		t.Fatalf("secrets.json missing: %v", err)
	}
	var enc models.EncryptedSecretData
	if err := json.Unmarshal(secData, &enc); err != nil {
		t.Fatalf("secrets.json not loadable: %v", err)
	}
	if enc.Version != models.SecretVersionCurrent {
		t.Errorf("secrets.json version = %d, want %d", enc.Version, models.SecretVersionCurrent)
	}
	plaintext, err := secretcrypto.DecryptAES(key, enc.Ciphertext, enc.Nonce)
	if err != nil {
		t.Fatalf("secrets.json does not decrypt with master.key: %v", err)
	}
	var providerKeys map[string][]models.SecretEntry
	if err := json.Unmarshal(plaintext, &providerKeys); err != nil {
		t.Fatalf("decrypted secrets unmarshal: %v", err)
	}
	if len(providerKeys) != 0 {
		t.Errorf("expected empty secrets, got %d providers", len(providerKeys))
	}
}

func TestSeedDefaults_Idempotent(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults 1: %v", err)
	}

	before := map[string]string{}
	for name, path := range map[string]string{
		"settings.yml":  p.ConfigFile(),
		"registry.json": p.RegistryFile(),
		"master.key":    p.MasterKeyFile(),
		"secrets.json":  p.SecretsFile(),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		before[name] = string(b)
	}

	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults 2: %v", err)
	}

	for name, path := range map[string]string{
		"settings.yml":  p.ConfigFile(),
		"registry.json": p.RegistryFile(),
		"master.key":    p.MasterKeyFile(),
		"secrets.json":  p.SecretsFile(),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(b) != before[name] {
			t.Errorf("%s was overwritten by a second SeedDefaults", name)
		}
	}
}

func TestSeedDefaults_FileModes(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	for name, path := range map[string]string{
		"master.key":      p.MasterKeyFile(),
		"master.key.hash": p.MasterKeyHashFile(),
		"secrets.json":    p.SecretsFile(),
		"settings.yml":    p.ConfigFile(),
		"registry.json":   p.RegistryFile(),
	} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 0600", name, fi.Mode().Perm())
		}
	}
}

func TestSeedDefaults_EnvMasterKeySkipsKeyFile(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvMasterKey, mustHex32(t, 1))

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if _, err := os.Stat(p.MasterKeyFile()); !os.IsNotExist(err) {
		t.Error("master.key should not be created when LLM_MASTER_KEY is set")
	}
	key, err := p.LoadMasterKey()
	if err != nil {
		t.Fatalf("LoadMasterKey: %v", err)
	}
	if hex.EncodeToString(key) != os.Getenv(EnvMasterKey) {
		t.Error("LoadMasterKey did not return the env key")
	}
}

func TestLoadMasterKey_EnvInvalid(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvMasterKey, "not-hex")
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := p.LoadMasterKey(); err == nil {
		t.Fatal("expected error for invalid LLM_MASTER_KEY")
	}
}

func TestLoadMasterKey_FileBackfillHash(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	// Remove the hash to simulate a legacy key without one.
	if err := os.Remove(p.MasterKeyHashFile()); err != nil {
		t.Fatal(err)
	}
	key, err := p.LoadMasterKey()
	if err != nil {
		t.Fatalf("LoadMasterKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key len = %d, want 32", len(key))
	}
	if _, err := os.Stat(p.MasterKeyHashFile()); err != nil {
		t.Errorf("hash should have been backfilled: %v", err)
	}
}

func TestLoadMasterKey_TamperedHash(t *testing.T) {
	resetEnv(t)
	t.Setenv("HOME", t.TempDir())
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if err := os.WriteFile(p.MasterKeyHashFile(), []byte("deadbeef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.LoadMasterKey(); err == nil {
		t.Fatal("expected integrity error for tampered hash")
	} else if !strings.Contains(err.Error(), "integrity") {
		t.Errorf("expected integrity error, got: %v", err)
	}
}

func TestPaths_Accessors(t *testing.T) {
	cfg := "/cfg"
	data := "/data"
	p := Paths{ConfigDir: cfg, DataDir: data}

	cases := map[string]string{
		"ConfigFile":    p.ConfigFile(),
		"RegistryFile":  p.RegistryFile(),
		"MasterKeyFile": p.MasterKeyFile(),
		"SecretsFile":   p.SecretsFile(),
		"DatabaseFile":  p.DatabaseFile(),
		"TemplatesDir":  p.TemplatesDir(),
		"MetadataDir":   p.MetadataDir(),
		"RunsDir":       p.RunsDir(),
		"LogsDir":       p.LogsDir(),
	}
	for name, got := range cases {
		if got == "" {
			t.Errorf("%s() returned empty", name)
		}
	}
	if p.ConfigFile() != filepath.Join(cfg, models.SettingsFilename) {
		t.Error("ConfigFile should live in ConfigDir")
	}
	if p.SecretsFile() != filepath.Join(cfg, models.SecretsFilename) {
		t.Error("SecretsFile should live in ConfigDir (next to master.key)")
	}
	if p.MasterKeyFile() != filepath.Join(cfg, "master.key") {
		t.Error("MasterKeyFile should live in ConfigDir")
	}
	if !strings.HasPrefix(p.MetadataDir(), data) {
		t.Error("MetadataDir should live under DataDir")
	}
}
