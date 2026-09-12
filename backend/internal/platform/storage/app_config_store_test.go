package storage

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"llm-proxy/models"
)

func TestSystemConfigView_Projection(t *testing.T) {
	mgr, _ := resetTestManager(t)

	if err := mgr.System().Update(func(sys *models.SystemConfig) error {
		sys.Server.Bind = "0.0.0.0:4001"
		sys.Server.ModelHost = "127.0.0.1"
		sys.Server.IdleTimeoutSecs = 1234
		sys.Server.Environment = map[string]string{"KEY": "VAL"}
		enabled := true
		sys.Server.RunLogging = &models.RunLoggingConfig{Enabled: enabled}
		sys.WorkspacesDir = "/tmp/ws"
		return nil
	}); err != nil {
		t.Fatalf("System().Update: %v", err)
	}

	sys := mgr.System().Get()
	if sys.Server.Bind != "0.0.0.0:4001" {
		t.Errorf("bind mismatch: %q", sys.Server.Bind)
	}
	if sys.Server.ModelHost != "127.0.0.1" {
		t.Errorf("model_host mismatch: %q", sys.Server.ModelHost)
	}
	if sys.Server.IdleTimeoutSecs != 1234 {
		t.Errorf("idle_timeout mismatch: %d", sys.Server.IdleTimeoutSecs)
	}
	if sys.Server.Environment["KEY"] != "VAL" {
		t.Errorf("environment not projected: %+v", sys.Server.Environment)
	}
	if sys.Server.RunLogging == nil || !sys.Server.RunLogging.Enabled {
		t.Errorf("run_logging not projected: %+v", sys.Server.RunLogging)
	}
	if sys.WorkspacesDir != "/tmp/ws" {
		t.Errorf("workspaces_dir mismatch: %q", sys.WorkspacesDir)
	}
}

func TestUserSettingsView_Projection(t *testing.T) {
	mgr, _ := resetTestManager(t)

	if err := mgr.Settings().Update(func(s *models.UserSettings) error {
		s.Local.ModelDir = "/models"
		s.ModelOverrides = map[string]models.ModelOverride{"m1": {MaxSteps: 5}}
		enabled := true
		s.RunOutput = &models.RunLoggingConfig{Enabled: enabled}
		return nil
	}); err != nil {
		t.Fatalf("Settings().Update: %v", err)
	}

	set := mgr.Settings().Get()
	if set.Local.ModelDir != "/models" {
		t.Errorf("model_dir mismatch: %q", set.Local.ModelDir)
	}
	if set.ModelOverrides["m1"].MaxSteps != 5 {
		t.Errorf("model_overrides not projected: %+v", set.ModelOverrides)
	}
	if set.RunOutput == nil || !set.RunOutput.Enabled {
		t.Errorf("run_output not projected: %+v", set.RunOutput)
	}
}

func TestHostSettingsView_Projection(t *testing.T) {
	mgr, _ := resetTestManager(t)

	if err := mgr.HostSettings().Update(func(hs *models.HostSettings) error {
		hs.Sandboxing.Enabled = false
		hs.Sandboxing.MaxMemoryMB = 512
		return nil
	}); err != nil {
		t.Fatalf("HostSettings().Update: %v", err)
	}

	hs := mgr.HostSettings().Get()
	if hs.Sandboxing.Enabled {
		t.Error("sandboxing.enabled should be false")
	}
	if hs.Sandboxing.MaxMemoryMB != 512 {
		t.Errorf("max_memory_mb mismatch: %d", hs.Sandboxing.MaxMemoryMB)
	}
}

func TestFacadeViews_OnChangeFiltersCrossTalk(t *testing.T) {
	mgr, _ := resetTestManager(t)

	var systemFired, settingsFired, hostFired int32
	mgr.System().OnChange(func(models.SystemConfig) { atomic.AddInt32(&systemFired, 1) })
	mgr.Settings().OnChange(func(models.UserSettings) { atomic.AddInt32(&settingsFired, 1) })
	mgr.HostSettings().OnChange(func(models.HostSettings) { atomic.AddInt32(&hostFired, 1) })

	// A host-only write must not fire the system/settings subscribers.
	if err := mgr.HostSettings().Update(func(hs *models.HostSettings) error {
		hs.Sandboxing.MaxMemoryMB = 512
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&systemFired); got != 0 {
		t.Errorf("system OnChange fired on host write: %d", got)
	}
	if got := atomic.LoadInt32(&settingsFired); got != 0 {
		t.Errorf("settings OnChange fired on host write: %d", got)
	}
	if got := atomic.LoadInt32(&hostFired); got != 1 {
		t.Errorf("host OnChange fired %d times, want 1", got)
	}

	// A system write fires only the system subscriber.
	if err := mgr.System().Update(func(sys *models.SystemConfig) error {
		sys.Server.ModelHost = "http://localhost"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&systemFired); got != 1 {
		t.Errorf("system OnChange fired %d times after system write, want 1", got)
	}
	if got := atomic.LoadInt32(&settingsFired); got != 0 {
		t.Errorf("settings OnChange fired on system write: %d", got)
	}

	// A settings write fires only the settings subscriber.
	if err := mgr.Settings().Update(func(s *models.UserSettings) error {
		s.Local.ModelDir = "/models"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&settingsFired); got != 1 {
		t.Errorf("settings OnChange fired %d times after settings write, want 1", got)
	}
	if got := atomic.LoadInt32(&systemFired); got != 1 {
		t.Errorf("system OnChange fired again on settings write: %d", got)
	}
}

func TestFacadeViews_GetDeepCopies(t *testing.T) {
	mgr, _ := resetTestManager(t)

	if err := mgr.System().Update(func(sys *models.SystemConfig) error {
		sys.Server.Environment = map[string]string{"A": "1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Mutating the returned copy must not leak into the live document (C1).
	got := mgr.System().Get()
	got.Server.Environment["A"] = "2"

	if live := mgr.System().Get().Server.Environment["A"]; live != "1" {
		t.Errorf("mutation leaked into live doc: %q", live)
	}
}

func TestAppConfigStore_MergesDefaultsForMissingKeys(t *testing.T) {
	mgr, p := resetTestManager(t)
	// Overwrite the seeded settings.yml with one missing the sandboxing and
	// server.bind keys (as a pre-relocation file would), then reload through
	// LoadAll which applies the default-merge.
	if err := os.WriteFile(p.ConfigFile(), []byte("local:\n  model_dir: /models\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	def := models.DefaultAppConfig()
	if !mgr.HostSettings().Get().Sandboxing.Enabled {
		t.Errorf("missing sandboxing key zeroed Enabled; want default %v", def.Sandboxing.Enabled)
	}
	if mgr.Settings().Get().Local.ModelDir != "/models" {
		t.Errorf("persisted value not preserved: %q", mgr.Settings().Get().Local.ModelDir)
	}
	if mgr.System().Get().Server.Bind != def.Server.Bind {
		t.Errorf("missing server.bind not filled from default: %q", mgr.System().Get().Server.Bind)
	}
}

// TestRunLoggingDefaultAndBackfill guards the regression where the config
// relocation flipped the shipped run_logging default from true to false,
// silently disabling per-run automation output. The default must be true, a
// settings.yml missing the key must be backfilled to the default, and an
// explicit persisted value must never be clobbered.
func TestRunLoggingDefaultAndBackfill(t *testing.T) {
	if !models.DefaultRunLoggingConfig().Enabled {
		t.Fatal("DefaultRunLoggingConfig().Enabled must be true (shipped default); run_logging regression")
	}
	if !models.DefaultAppConfig().RunLogging.Enabled {
		t.Fatal("DefaultAppConfig().RunLogging must be enabled by default")
	}

	t.Run("missing key backfilled from default", func(t *testing.T) {
		mgr, p := resetTestManager(t)
		// settings.yml predating run_logging (legacy layout kept it in config.json).
		if err := os.WriteFile(p.ConfigFile(), []byte("local:\n  model_dir: /models\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := mgr.LoadAll(); err != nil {
			t.Fatal(err)
		}
		if sys := mgr.System().Get(); sys.Server.RunLogging == nil || !sys.Server.RunLogging.Enabled {
			t.Errorf("run_logging not backfilled to enabled default: %+v", sys.Server.RunLogging)
		}
	})

	t.Run("explicit persisted value never clobbered", func(t *testing.T) {
		mgr, _ := resetTestManager(t)
		if err := mgr.System().Update(func(sys *models.SystemConfig) error {
			sys.Server.RunLogging = &models.RunLoggingConfig{Enabled: false}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		// Reload from disk: an explicit disabled setting survives the default-merge.
		if err := mgr.appConfigStore.Load(); err != nil {
			t.Fatal(err)
		}
		if sys := mgr.System().Get(); sys.Server.RunLogging == nil || sys.Server.RunLogging.Enabled {
			t.Errorf("explicit run_logging.enabled=false clobbered by default merge: %+v", sys.Server.RunLogging)
		}
	})
}

func TestSandboxingNewFieldsDefaultAndBackfill(t *testing.T) {
	t.Run("legacy file keeps jail on and network undecided-allowed", func(t *testing.T) {
		mgr, p := resetTestManager(t)
		// Simulate an upgrade: overwrite with a settings.yml that predates the
		// filesystem/network keys (the D6 additive-config hazard).
		legacy := "server:\n  bind: \"127.0.0.1:9000\"\nsandboxing:\n  enabled: true\n  maxstoragegb: 2\n  maxmemorymb: 256\n  functional: false\n"
		if err := os.WriteFile(p.ConfigFile(), []byte(legacy), 0600); err != nil {
			t.Fatal(err)
		}
		if err := mgr.LoadAll(); err != nil {
			t.Fatal(err)
		}

		hs := mgr.HostSettings().Get().Sandboxing
		if !hs.Enabled {
			t.Error("legacy enabled:true lost by merge")
		}
		if hs.Filesystem != nil {
			t.Errorf("filesystem must stay undecided (nil) for legacy files, got %+v", hs.Filesystem)
		}
		if hs.Network != nil {
			t.Errorf("network must stay undecided (nil) for legacy files — an explicit false would silently flip existing installs off, got %+v", hs.Network)
		}
		if !hs.FilesystemEnabled() {
			t.Error("effective filesystem jail must be ON for legacy files")
		}
		if !hs.NetworkAllowed() {
			t.Error("effective network must be ALLOWED for legacy files until the operator decides")
		}
		if hs.NetworkDecided() {
			t.Error("legacy network must report undecided (migration banner state)")
		}

		// The merge must NOT have rewritten the file with a network key.
		raw, err := os.ReadFile(p.ConfigFile())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "network:") {
			t.Errorf("legacy file was rewritten with a network key:\n%s", raw)
		}
	})

	t.Run("fresh install persists explicit defaults", func(t *testing.T) {
		mgr, p := resetTestManager(t) // empty dir → LoadAll writes merged defaults
		hs := mgr.HostSettings().Get().Sandboxing
		if !hs.FilesystemEnabled() {
			t.Error("fresh install must default filesystem jail ON")
		}
		if hs.NetworkAllowed() {
			t.Error("fresh install must default agent network OFF (D1)")
		}
		raw, err := os.ReadFile(p.ConfigFile())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "network: false") {
			t.Errorf("fresh settings.yml must persist explicit network:false (D1 default-off):\n%s", raw)
		}
		if strings.Contains(string(raw), "filesystem:") {
			t.Errorf("fresh settings.yml needs no filesystem key (nil resolves to ON):\n%s", raw)
		}
	})

	t.Run("explicit persisted value never clobbered", func(t *testing.T) {
		mgr, p := resetTestManager(t)
		fsOn, netOff := true, false
		if err := mgr.HostSettings().Update(func(hs *models.HostSettings) error {
			hs.Sandboxing.Filesystem = &fsOn
			hs.Sandboxing.Network = &netOff
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := mgr.LoadAll(); err != nil { // reload from disk
			t.Fatal(err)
		}
		hs := mgr.HostSettings().Get().Sandboxing
		if hs.Filesystem == nil || !*hs.Filesystem {
			t.Error("explicit filesystem:true clobbered by default merge")
		}
		if hs.Network == nil || *hs.Network {
			t.Error("explicit network:false clobbered by default merge")
		}
		raw, err := os.ReadFile(p.ConfigFile())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "network: false") {
			t.Errorf("explicit network:false not persisted:\n%s", raw)
		}
	})
}

func TestSandboxingSectionPresenceSemantics(t *testing.T) {
	t.Run("explicit enabled:false survives reload with numeric backfill", func(t *testing.T) {
		mgr, p := resetTestManager(t)
		// Section present but only the master switch set: this must NOT be
		// merged back to defaults (which would silently re-enable terminals).
		explicit := "sandboxing:\n  enabled: false\n"
		if err := os.WriteFile(p.ConfigFile(), []byte(explicit), 0600); err != nil {
			t.Fatal(err)
		}
		if err := mgr.LoadAll(); err != nil {
			t.Fatal(err)
		}
		hs := mgr.HostSettings().Get().Sandboxing
		if hs.Enabled {
			t.Error("explicit enabled:false was clobbered by the defaults merge")
		}
		if hs.MaxStorageGB != 2 || hs.MaxMemoryMB != 2048 {
			t.Errorf("per-key numeric backfill missing: storage=%d memory=%d", hs.MaxStorageGB, hs.MaxMemoryMB)
		}
		if hs.Network != nil || hs.Filesystem != nil {
			t.Errorf("undecided *bool keys must stay nil, got net=%v fs=%v", hs.Network, hs.Filesystem)
		}
	})

	t.Run("absent sandboxing section takes defaults", func(t *testing.T) {
		mgr, p := resetTestManager(t)
		noSection := "server:\n  bind: \"127.0.0.1:9000\"\n"
		if err := os.WriteFile(p.ConfigFile(), []byte(noSection), 0600); err != nil {
			t.Fatal(err)
		}
		if err := mgr.LoadAll(); err != nil {
			t.Fatal(err)
		}
		hs := mgr.HostSettings().Get().Sandboxing
		if !hs.Enabled {
			t.Error("absent section must resolve to the default enabled master")
		}
		if hs.MaxStorageGB != 2 || hs.MaxMemoryMB != 2048 {
			t.Errorf("absent section must take numeric defaults, got storage=%d memory=%d", hs.MaxStorageGB, hs.MaxMemoryMB)
		}
	})
}

func TestGetProjected_DeepCopiesProjection(t *testing.T) {
	dir := t.TempDir()
	s := NewStore[models.AppConfig](dir + "/settings.yml")
	cfg := models.DefaultAppConfig()
	cfg.Server.Environment = map[string]string{"A": "1"}
	if err := s.Update(func(c *models.AppConfig) error {
		*c = cfg
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	proj := GetProjected(s, func(c *models.AppConfig) models.HostSettings {
		return models.HostSettings{Sandboxing: c.Sandboxing}
	})
	proj.Sandboxing.Enabled = !proj.Sandboxing.Enabled

	if live := s.Get().Sandboxing.Enabled; live == proj.Sandboxing.Enabled {
		t.Error("GetProjected returned a value aliasing the live doc")
	}
}
