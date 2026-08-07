package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/models"
)

func seededPathsForTest(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	return p
}

func TestDataManager_LoadAllFresh(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	// Fresh seed → seeded defaults loaded (Bind set by SeedDefaults), empty
	// registry, empty secrets.
	if sys := mgr.System().Get(); sys.Server.Bind == "" {
		t.Errorf("expected seeded default bind on fresh seed, got: %+v", sys)
	}
	if reg := mgr.Registry().Get(); reg.PrimaryModel != "" || len(reg.Catalogue) != 0 {
		t.Errorf("expected empty registry, got %+v", reg)
	}
	if keys := mgr.Secrets().GetProviderKeys("openai"); len(keys) != 0 {
		t.Errorf("expected empty secrets, got %+v", keys)
	}
	if mgr.ConfigDir() != p.ConfigDir || mgr.DataDir() != p.DataDir {
		t.Error("ConfigDir/DataDir accessors mismatch")
	}
}

func TestDataManager_LoadAllPopulated(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}

	if err := mgr.Registry().Update(func(r *models.RegistryData) error {
		r.PrimaryModel = "alpha"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Secrets().SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}

	// Reload through a fresh manager.
	mgr2, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if got := mgr2.Registry().Get().PrimaryModel; got != "alpha" {
		t.Errorf("PrimaryModel = %q, want alpha", got)
	}
	if got := mgr2.Secrets().GetProviderKeys("openai"); len(got) != 1 || got[0].Key != "sk-x" {
		t.Errorf("secrets reload mismatch: %+v", got)
	}
}

func TestDataManager_MissingMasterKeyIsLoud(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if _, err := NewDataManager(p); err == nil {
		t.Fatal("expected NewDataManager to fail when the master key is missing")
	} else if !strings.Contains(err.Error(), "master key") {
		t.Errorf("expected master key error, got: %v", err)
	}
}

func TestDataManager_WorkspacesDirResolution(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, _ := NewDataManager(p)
	mgr.LoadAll()

	// Unset → default {repoRoot}/workspaces where {repoRoot} is found by
	// walking up from the current working directory to backend/go.mod.
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if root == "" {
		t.Fatal("expected a discoverable repo root from the test working directory")
	}
	got := mgr.WorkspacesDir()
	want := filepath.Join(root, models.WorkspacesDirName)
	if got != want {
		t.Errorf("default WorkspacesDir = %q, want %q", got, want)
	}

	// Absolute configured value wins.
	if err := mgr.System().Update(func(s *models.SystemConfig) error {
		s.WorkspacesDir = "/abs/ws"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.WorkspacesDir(); got != "/abs/ws" {
		t.Errorf("absolute WorkspacesDir = %q", got)
	}

	// Relative configured value resolves against the data root.
	if err := mgr.System().Update(func(s *models.SystemConfig) error {
		s.WorkspacesDir = "rel-ws"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.WorkspacesDir(); got != filepath.Join(p.DataDir, "rel-ws") {
		t.Errorf("relative WorkspacesDir = %q", got)
	}
}

// TestFindRepoRoot verifies the repo-root walker anchors to the nearest
// ancestor containing backend/go.mod, independent of how deep the process
// working directory is inside the repo.
func TestFindRepoRoot(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "my-project")
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	marker := filepath.Join(repo, "backend", "go.mod")
	if err := os.WriteFile(marker, []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "backend", "internal", "core"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd) //nolint:errcheck

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{name: "from backend dir", dir: filepath.Join(repo, "backend"), want: repo},
		{name: "from nested dir", dir: filepath.Join(repo, "backend", "internal", "core"), want: repo},
		{name: "from repo root", dir: repo, want: repo},
		{name: "from unrelated dir (no marker)", dir: base, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Chdir(tc.dir); err != nil {
				t.Fatalf("chdir %s: %v", tc.dir, err)
			}
			got, err := findRepoRoot()
			if err != nil {
				t.Fatalf("findRepoRoot: %v", err)
			}
			if got != tc.want {
				t.Errorf("findRepoRoot from %q = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// eventually polls cond until it returns true or the timeout elapses.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}

// TestDataManager_WatcherLifecycle asserts C2/C3 lifecycle: Close waits for the
// watcher goroutine to exit and the watcher can be restarted afterwards.
func TestDataManager_WatcherLifecycle(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := mgr.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := mgr.Watch(ctx); err == nil {
		t.Fatal("expected second Watch to fail while running")
	}

	// Close must wait for the goroutine to exit.
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cancel()

	// Restartable after Close.
	if err := mgr.Watch(context.Background()); err != nil {
		t.Fatalf("re-Watch: %v", err)
	}
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
	// Close is idempotent.
	if err := mgr.Close(context.Background()); err != nil {
		t.Fatalf("Close 3: %v", err)
	}
}

// TestDataManager_WatcherCoalescesEvents asserts a burst of writes for the same
// file still converges on the final value without wedging the watcher (C3).
func TestDataManager_WatcherCoalescesEvents(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Watch(context.Background()); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	// Burst of writes to registry.json in quick succession.
	registryPath := filepath.Join(p.DataDir, models.RegistryFilename)
	for i := 0; i < 5; i++ {
		reg := models.RegistryData{PrimaryModel: fmt.Sprintf("m%d", i)}
		data, _ := json.Marshal(reg)
		if err := os.WriteFile(registryPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The debounced reload must converge on the final write.
	eventually(t, 3*time.Second, func() bool {
		return mgr.Registry().Get().PrimaryModel == "m4"
	}, "final registry write did not load")
}

// TestDataManager_FreshInstallSecretsSave mirrors the production boot path
// (SeedDefaults -> NewDataManager -> LoadAll) on a completely empty directory and
// asserts that saving an API key succeeds with no decrypt error. This guards the
// "wipe the config folder and launch" scenario: a fresh install must produce a
// decryptable (empty) secrets.json keyed to the very master key the runtime
// loads, so the first credential write never hits a MAC failure.
func TestDataManager_FreshInstallSecretsSave(t *testing.T) {
	p := seededPathsForTest(t)

	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	if err := mgr.Secrets().SetProviderKeys("nvidia", []models.APIKeyItem{
		{ID: "k1", Name: "primary", Key: "nvapi-xxxx"},
	}); err != nil {
		t.Fatalf("first key save on fresh install failed: %v", err)
	}

	got := mgr.Secrets().GetProviderKeys("nvidia")
	if len(got) != 1 || got[0].Key != "nvapi-xxxx" {
		t.Errorf("saved key not readable back: %+v", got)
	}
}

func TestDataManager_WatcherReloadsExternalWrite(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Watch(context.Background()); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	reg := models.RegistryData{PrimaryModel: "external"}
	data, _ := json.Marshal(reg)
	registryPath := filepath.Join(p.DataDir, models.RegistryFilename)
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	eventually(t, 3*time.Second, func() bool {
		return mgr.Registry().Get().PrimaryModel == "external"
	}, "watcher did not reload externally-written registry.json")
}

func TestDataManager_WatcherIgnoresTempFiles(t *testing.T) {
	p := seededPathsForTest(t)
	mgr, err := NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Watch(context.Background()); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	// A temp file matching the WriteAtomic pattern must not trigger any reload.
	tmp := filepath.Join(p.DataDir, models.RegistryFilename+".123456789.tmp")
	if err := os.WriteFile(tmp, []byte(`{"primary_model":"temp"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if got := mgr.Registry().Get().PrimaryModel; got != "" {
		t.Errorf("temp file write triggered a reload: primary_model = %q", got)
	}
}

// TestMergeAppConfigDefaults verifies that absent top-level fields are backfilled
// from DefaultAppConfig without ever clobbering persisted values. In particular a
// partially-populated metrics section must keep its binary/sysfs/sampling fields
// even when the GPU provider is empty (regression for the wholesale struct
// replacement that dropped those fields).
func TestMergeAppConfigDefaults(t *testing.T) {
	def := models.DefaultAppConfig()

	t.Run("fully absent metrics backfilled", func(t *testing.T) {
		c := models.AppConfig{Server: models.AppServerConfig{Bind: "127.0.0.1:9000"}}
		got := mergeAppConfigDefaults(def, c)
		if got.Metrics.GPU.Provider != "auto" {
			t.Errorf("expected default GPU provider backfilled, got %q", got.Metrics.GPU.Provider)
		}
		if got.Server.Bind != "127.0.0.1:9000" {
			t.Errorf("persisted bind clobbered: %q", got.Server.Bind)
		}
	})

	t.Run("partial metrics keeps persisted fields", func(t *testing.T) {
		c := models.AppConfig{
			Metrics: models.MetricsConfig{
				GPU: models.GPUConfig{
					Binary:    "/usr/bin/nvidia-smi",
					Index:     2,
					SysfsPath: "/sys/class/drm/card1",
				},
				GPUSampleIntervalSec: 7,
				GPUSmoothingAlpha:    0.5,
			},
		}
		got := mergeAppConfigDefaults(def, c)
		if got.Metrics.GPU.Binary != "/usr/bin/nvidia-smi" {
			t.Errorf("persisted binary lost: %q", got.Metrics.GPU.Binary)
		}
		if got.Metrics.GPU.Index != 2 {
			t.Errorf("persisted index lost: %d", got.Metrics.GPU.Index)
		}
		if got.Metrics.GPU.SysfsPath != "/sys/class/drm/card1" {
			t.Errorf("persisted sysfs path lost: %q", got.Metrics.GPU.SysfsPath)
		}
		if got.Metrics.GPUSampleIntervalSec != 7 {
			t.Errorf("persisted sample interval lost: %d", got.Metrics.GPUSampleIntervalSec)
		}
		if got.Metrics.GPUSmoothingAlpha != 0.5 {
			t.Errorf("persisted smoothing alpha lost: %v", got.Metrics.GPUSmoothingAlpha)
		}
		if got.Metrics.GPU.Provider != "auto" {
			t.Errorf("default provider not backfilled: %q", got.Metrics.GPU.Provider)
		}
	})

	t.Run("explicit values never overwritten", func(t *testing.T) {
		c := models.AppConfig{
			Server: models.AppServerConfig{
				Bind:            "0.0.0.0:8080",
				ModelHost:       "10.0.0.5",
				IdleTimeoutSecs: 60,
			},
			Metrics:    models.MetricsConfig{GPU: models.GPUConfig{Provider: "none"}},
			Sandboxing: models.HostSandboxingConfig{Enabled: false, MaxMemoryMB: 512},
		}
		got := mergeAppConfigDefaults(def, c)
		if got.Server.Bind != "0.0.0.0:8080" || got.Server.ModelHost != "10.0.0.5" || got.Server.IdleTimeoutSecs != 60 {
			t.Errorf("server section was clobbered: %+v", got.Server)
		}
		if got.Metrics.GPU.Provider != "none" {
			t.Errorf("explicit GPU provider overwritten: %q", got.Metrics.GPU.Provider)
		}
		if got.Sandboxing.Enabled {
			t.Errorf("explicit sandboxing disabled was overwritten: %+v", got.Sandboxing)
		}
		if got.Sandboxing.MaxMemoryMB != 512 {
			t.Errorf("explicit MaxMemoryMB lost: %d", got.Sandboxing.MaxMemoryMB)
		}
	})
}
