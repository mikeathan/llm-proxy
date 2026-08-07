package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/models"
)

func resetTestManager(t *testing.T) (*DataManager, paths.Paths) {
	t.Helper()
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	return mgr, p
}

func TestFactoryReset_ReseedsDefaults(t *testing.T) {
	mgr, p := resetTestManager(t)

	// Write a provider key so we can prove reset clears it.
	if err := mgr.Secrets().SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}

	res, err := mgr.FactoryReset()
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	if !res.KeyRegenerated {
		t.Error("expected key to be regenerated in file-managed mode")
	}

	// settings.yml + registry.json load; secrets round-trip empty.
	if _, err := os.Stat(p.ConfigFile()); err != nil {
		t.Errorf("settings.yml missing: %v", err)
	}
	if _, err := os.Stat(p.RegistryFile()); err != nil {
		t.Errorf("registry.json missing: %v", err)
	}
	if got := mgr.Secrets().GetProviderKeys("openai"); len(got) != 0 {
		t.Errorf("secrets not cleared: %+v", got)
	}

	// New master key verifies against its hash.
	if err := p.EnforcePermissions(); err != nil {
		t.Errorf("permissions after reset: %v", err)
	}
}

func TestFactoryReset_NewKeyDiffersFromOld(t *testing.T) {
	mgr, _ := resetTestManager(t)

	oldKey, err := mgr.paths.LoadMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.FactoryReset(); err != nil {
		t.Fatal(err)
	}
	newKey, err := mgr.paths.LoadMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(oldKey) == string(newKey) {
		t.Error("expected a different master key after reset")
	}
}

func TestFactoryReset_EnvKeyModeReusesKey(t *testing.T) {
	dir := t.TempDir()
	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	t.Setenv(paths.EnvMasterKey, key)

	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	res, err := mgr.FactoryReset()
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	if res.KeyRegenerated {
		t.Error("env-managed key must not be regenerated")
	}
	if !res.KeyExternallyManaged {
		t.Error("expected key_externally_managed=true")
	}
	// secrets still round-trip under the env key.
	if err := mgr.Secrets().SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Secrets().GetProviderKeys("openai"); len(got) != 1 {
		t.Errorf("secrets round-trip failed in env mode: %+v", got)
	}
}

func TestFactoryReset_SecretsConcurrentAccess(t *testing.T) {
	mgr, _ := resetTestManager(t)
	if err := mgr.Secrets().SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = mgr.Secrets().GetProviderKeys("openai")
				}
			}
		}()
	}

	// FactoryReset rebuilds secretsStore with a new key while readers above
	// hold the store via Secrets(). Under -race this must be clean (Fix: the
	// pointer swap is guarded by a mutex).
	if _, err := mgr.FactoryReset(); err != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("FactoryReset: %v", err)
	}
	close(stop)
	wg.Wait()
}

func TestFactoryReset_SwapFailureRollsBack(t *testing.T) {
	cfgDir := t.TempDir()
	dataDir := t.TempDir()
	p := paths.Paths{ConfigDir: cfgDir, DataDir: dataDir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if err := mgr.Secrets().SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}

	orig, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}

	// With the single-root layout all targets live in ConfigDir. Make it
	// unwritable so the first swap attempt fails and triggers rollback. The test
	// asserts state stays consistent even when FactoryReset cannot complete.
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cfgDir, 0o700)

	if _, err := mgr.FactoryReset(); err == nil {
		t.Fatal("expected FactoryReset to fail when a swap target cannot be written")
	}

	// settings.yml was swapped before the failure; it must be rolled back.
	after, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(orig) {
		t.Error("settings.yml was not restored after a failed swap")
	}
	// Key/ciphertext must stay consistent: the original secret still decrypts.
	if got := mgr.Secrets().GetProviderKeys("openai"); len(got) != 1 {
		t.Errorf("secrets lost after failed reset: %+v", got)
	}
}

func TestClearRuntimeData_DeletesSessionsRunsLogs(t *testing.T) {
	mgr, p := resetTestManager(t)

	// Create runtime artifacts.
	wsMeta := filepath.Join(p.MetadataDir(), "ws1")
	if err := os.MkdirAll(filepath.Join(wsMeta, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsMeta, "process.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := p.RunsDir()
	if err := os.MkdirAll(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	logs := p.LogsDir()
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := mgr.ClearRuntimeData(); err != nil {
		t.Fatalf("ClearRuntimeData: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wsMeta, "sessions")); !os.IsNotExist(err) {
		t.Error("sessions not removed")
	}
	if _, err := os.Stat(filepath.Join(wsMeta, "process.log")); !os.IsNotExist(err) {
		t.Error("process.log not removed")
	}
	if _, err := os.Stat(runs); err != nil {
		t.Error("runs/ should be recreated, not error")
	}
	if _, err := os.Stat(logs); err != nil {
		t.Error("logs/ should be recreated, not error")
	}
	// Config/secrets/db untouched.
	if _, err := os.Stat(p.ConfigFile()); err != nil {
		t.Error("settings.yml removed by clear-runtime-data")
	}
	if _, err := os.Stat(p.SecretsFile()); err != nil {
		t.Error("secrets.json removed by clear-runtime-data")
	}
}

func TestWipeout_DeletesRootAndWorkspaces(t *testing.T) {
	mgr, p := resetTestManager(t)

	ws := t.TempDir()
	if err := mgr.System().Update(func(sys *models.SystemConfig) error {
		sys.WorkspacesDir = ws
		return nil
	}); err != nil {
		t.Fatalf("set workspaces dir: %v", err)
	}

	// Place recognizable artifacts in both targets.
	if err := os.WriteFile(filepath.Join(ws, "user-file.txt"), []byte("keep me gone"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.ConfigDir, "extra.txt"), []byte("gone"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := mgr.Wipeout()
	if err != nil {
		t.Fatalf("Wipeout: %v", err)
	}
	if res.RootDir != p.ConfigDir {
		t.Errorf("RootDir = %q, want %q", res.RootDir, p.ConfigDir)
	}
	if res.WorkspacesDir != ws {
		t.Errorf("WorkspacesDir = %q, want %q", res.WorkspacesDir, ws)
	}

	if _, err := os.Stat(p.ConfigDir); !os.IsNotExist(err) {
		t.Error("data root not removed")
	}
	if _, err := os.Stat(ws); !os.IsNotExist(err) {
		t.Error("workspaces dir not removed")
	}
}

func TestWipeout_RefusesUnsafeWorkspacesDir(t *testing.T) {
	mgr, p := resetTestManager(t)

	if err := mgr.System().Update(func(sys *models.SystemConfig) error {
		sys.WorkspacesDir = "/"
		return nil
	}); err != nil {
		t.Fatalf("set workspaces dir: %v", err)
	}

	if _, err := mgr.Wipeout(); err == nil {
		t.Fatal("expected Wipeout to refuse a workspaces dir of /")
	}

	// The data root must be left untouched when the operation is refused.
	if _, err := os.Stat(p.ConfigDir); err != nil {
		t.Errorf("data root removed despite refusal: %v", err)
	}
}

func TestValidateWipeTarget(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	safe := filepath.Join(home, ".config", "llm-proxy")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "filesystem root", path: "/", wantErr: true},
		{name: "empty path", path: "", wantErr: true},
		{name: "home dir", path: home, wantErr: true},
		{name: "home ancestor", path: filepath.Dir(home), wantErr: true},
		{name: "safe config root", path: safe, wantErr: false},
		{name: "safe workspaces leaf", path: filepath.Join(home, "llm-proxy", "workspaces"), wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWipeTarget(tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected %q to be refused", tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", tc.path, err)
			}
		})
	}
}
